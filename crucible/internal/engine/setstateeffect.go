package engine

//enginelint:allow game ability control effecthelpers card condition id zone valid defined parts effecteffect cloneeffect facedown amount

import (
	"fmt"
	"github.com/jczastkiewicz/crucible/internal/mana"
	"strconv"
	"strings"

	"github.com/jczastkiewicz/crucible/internal/carddb"
	"github.com/jczastkiewicz/crucible/internal/carddb/compile"
	"github.com/jczastkiewicz/crucible/internal/cardtype"
	"github.com/jczastkiewicz/crucible/internal/valid"
)

// setStateEffect is SetStateEffect.java for Mode$ Transform and TurnFaceUp:
// each targeted or Defined$ card -- or, with Choices$, Amount$ (default 1;
// at least MinAmount$) cards the activator picks among the matching
// permanents -- changes state. Transform flips a transforming double-faced
// permanent to its other face (CR 701.28) with a new timestamp (CR
// 613.7g); it does nothing to anything else, to a face-down permanent, or
// to one whose other face is not a permanent, and a permanent's own
// ability does nothing if the permanent has transformed since the ability
// went on the stack (CR 701.28f). TurnFaceUp turns a face-down permanent
// face up (CR 708.8) unless a Layer$ CantHappen Event$ TurnFaceUp
// replacement forbids it (canBeTurnedFaceUp), and runs the Event$ Transform
// and TurnFaceUp ReplaceWith$ lines once the face changed
// (faceChangeReplaced, replacementfaces.go). Optional$ asks first;
// RememberChanged$ remembers each changed card. Mode$ Flip (CR 709) is
// one-way and gives no timestamp, no Transformed trigger and no replacement
// (Card.java:716-745, g.flip). TurnFaceDown turns a face-up permanent face
// down (Card.java:762-767, turnFaceDown), ending the UntilFacedown copy
// effects it hosts; it runs no replacement or trigger. Mode$ Specialize
// (ADR-0050) swaps in the face of the source's chosen colour for good.
type setStateEffect struct{}

func (setStateEffect) Resolve(g *Game, a *Ability, controller PlayerController) error {
	if err := rejectUnreadCondition(a, a.Params.Name); err != nil {
		return err
	}
	if err := rejectParams(a, "SetState", "RevealFirst", "ValidNewFace", "NewState",
		"ETB"); err != nil {
		return err
	}
	source := g.Card(a.Source)
	if !subAbilityConditionMet(g, source, a.Amounts, a.Params) {
		return nil
	}
	mode, _ := a.Params.Param("Mode")
	if mode != "Transform" && mode != "TurnFaceUp" && mode != "Flip" && mode != "TurnFaceDown" && mode != "Specialize" {
		return fmt.Errorf("engine: SetState: Mode$ %q not resolvable yet", mode)
	}
	if mode == "Transform" && battlefieldStaticMode(g, "CantTransform") {
		return fmt.Errorf("engine: SetState: CantTransform statics not resolvable yet")
	}
	var cards []CardID
	if spec, ok := a.Params.Param("Choices"); ok {
		parsed := valid.Parse(spec)
		var choices []CardID
		for _, pid := range g.Players() {
			for _, id := range g.Zone(Battlefield, pid).Cards() {
				if Matches(g, g.Card(id), parsed, a.Controller, a.Source) {
					choices = append(choices, id)
				}
			}
		}
		amount, err := optionalAmount(g, a, "SetState", "Amount", 1)
		if err != nil {
			return err
		}
		if amount <= 0 {
			return nil
		}
		minCards, err := optionalAmount(g, a, "SetState", "MinAmount", amount)
		if err != nil {
			return err
		}
		if !hasParam(a, "Mandatory") {
			minCards = 0
		}
		if amount > len(choices) {
			amount = len(choices)
		}
		if minCards > amount {
			minCards = amount
		}
		cards = controller.ChooseCardsForEffect(g, a.Controller, a.Source, choices, minCards, amount)
		if err := checkChoice(cards, choices, minCards, amount); err != nil {
			return fmt.Errorf("engine: SetState: %w", err)
		}
	} else {
		var err error
		if cards, err = targetedOrDefinedCards(source, a.Params, a.refs()); err != nil {
			return fmt.Errorf("engine: SetState: %w", err)
		}
	}
	if mode == "TurnFaceDown" {
		for _, id := range cards {
			if err := g.turnFaceDownCheck(id); err != nil {
				return err
			}
		}
	}
	for _, id := range cards {
		c := g.Card(id)
		if mode == "Transform" && c.Zone != Battlefield {
			continue
		}
		if mode == "Transform" && id == a.Source && a.hasHostTransforms && c.Transforms != a.hostTransforms {
			continue
		}
		if hasParam(a, "Optional") && !controller.ConfirmEffect(g, a.Controller, a.Source) {
			return nil
		}
		changed := false
		switch {
		case mode == "Transform":
			changed = g.transform(id)
		case mode == "Flip":
			changed = g.flip(id)
		case mode == "Specialize":
			var err error
			if changed, err = g.specialize(id, source.Memory.ChosenColors()); err != nil {
				return err
			}
			source.Memory.SetChosenColors(0)
		case mode == "TurnFaceDown":
			if changed = g.turnFaceDown(id); changed {
				if err := g.setFaceDownState(a, source, c); err != nil {
					return err
				}
			}
		case c.IsFaceDown() && g.canBeTurnedFaceUp(id) && c.faceUpDef.Faces[0].Type.IsPermanent():
			c.turnFaceUp()
			// CR 613.7f: a permanent turned face up gets a new timestamp
			// (Card.java:891).
			g.timestamp++
			c.Timestamp = g.timestamp
			changed = true
		}
		if changed {
			// Card.java runs the Event$ Transform / TurnFaceUp replacements
			// after the new face is in place and before its trigger.
			switch mode {
			case "Transform":
				g.faceChangeReplaced(controller, "Transform", id)
				g.checkTransformedTriggers(controller, id)
			case "TurnFaceUp":
				g.faceChangeReplaced(controller, "TurnFaceUp", id)
				g.checkTurnedFaceUpTriggers(controller, id, a)
			case "Specialize":
				g.checkSpecializesTriggers(controller, id)
			}
		}
		if changed && hasParam(a, "RememberChanged") {
			source.Memory.Remember(CardEntity(id))
		}
	}
	return nil
}

// turnFaceDownCheck is whether id can be turned face down by this port, an
// error naming the shape it cannot: Card.changeCardState("TurnFaceDown")
// (Card.java:762-767) for a permanent, not a Room, under no copy effect but
// the UntilFacedown ones it hosts itself -- the face-down state takes the one
// Def slot those would otherwise have to share.
func (g *Game) turnFaceDownCheck(id CardID) error {
	c := g.Card(id)
	switch {
	case c.Zone != Battlefield:
		return fmt.Errorf("engine: SetState: TurnFaceDown outside the battlefield (%v) not resolvable yet", c.Zone)
	case c.IsRoomPermanent():
		return fmt.Errorf("engine: SetState: TurnFaceDown of a Room not resolvable yet")
	}
	for _, e := range c.copies {
		if e.permanent || e.life.duration != effectUntilFacedown || e.life.host != id {
			return fmt.Errorf("engine: SetState: TurnFaceDown of a permanent under another copy effect not resolvable yet")
		}
	}
	return nil
}

// turnFaceDown is Card.turnFaceDown (Card.java:841-863) for a permanent
// turnFaceDownCheck accepted: a face-up, single-faced permanent becomes a
// face-down 2/2 (CR 708.2a) with a new timestamp (CR 613.7f), and its
// facedown commands run -- the UntilFacedown copy effects it hosts end
// (endCopiesOnFaceDown), here before the face changes so the face-down state
// covers the card's own values. A face-down or double-faced permanent does
// not turn (Java's !isDoubleFaced and oldState checks). Reports whether it
// turned.
func (g *Game) turnFaceDown(id CardID) bool {
	c := g.Card(id)
	if c.IsFaceDown() || c.frontDef != nil {
		return false
	}
	// Card.isDoubleFaced reads the card's own rules, not a copied state.
	if own := c.UncopiedDef(); own != nil {
		switch own.SplitType {
		case carddb.SplitTransform, carddb.SplitMeld, carddb.SplitModal:
			return false
		}
	}
	g.endCopiesOnFaceDown(id)
	c.clearTextChange()
	c.faceUpDef = c.Def
	c.Def = faceDownDef(false)
	g.timestamp++
	c.Timestamp = g.timestamp
	return true
}

// flip is Card.changeCardState("Flip") (Card.java:716-745), CR 709.4: a
// permanent that has not flipped does, once. A flip card on the battlefield
// takes its flipped face, the way transform swaps Def; flipping gives no new
// timestamp. A face-down or copying permanent, and a card with no flipped
// face, only records that it flipped: Java's `flipped` flag is set while the
// state stays the copied or face-down one. A card with a flipped face that
// turns face up later would show it (Card.getFaceupCardStateName); this port
// has no turn-face-up path for a flip card that is face down, and turnFaceUp
// restores the unflipped Def.
func (g *Game) flip(id CardID) bool {
	return g.Flip(id)
}

// Flip is flip for a caller outside an effect: GameState's `|Flipped` entry
// (GameState.java:1349) puts a loaded card in its flipped state.
func (g *Game) Flip(id CardID) bool {
	c := g.Card(id)
	if c.flipped {
		return false
	}
	c.flipped = true
	if c.IsFaceDown() || len(c.copies) > 0 || c.frontDef != nil {
		return true
	}
	front := c.Def
	if front.SplitType != carddb.SplitFlip || front.Faces[1].Name == "" {
		return true
	}
	back := &compile.Card{Filename: front.Filename, Name: front.Faces[1].Name, SplitType: front.SplitType}
	back.Faces[0] = front.Faces[1]
	c.frontDef, c.Def = front, back
	return true
}

// Transform is transform for a caller outside an effect: GameState's
// `|Transformed` entry (GameState.java:1346) puts a loaded card on its back
// face, with no replacement run.
func (g *Game) Transform(id CardID) bool { return g.transform(id) }

// transform is Card.changeCardState("Transform"): a transforming
// double-faced permanent that is face up and whose other face is a
// permanent turns to that face, under a new timestamp.
func (g *Game) transform(id CardID) bool {
	c := g.Card(id)
	if c.IsFaceDown() {
		return false
	}
	// A permanent under a copy effect has the copied object's single face
	// only (CardFactory.getCloneStates' current-state branch), so it has no
	// back face to turn to. Java still flips its own backside flag here
	// (Card.changeCardState), so the underlying card shows its other face
	// once the copy ends; this port does not reproduce that (quirks.md).
	if len(c.copies) > 0 {
		return false
	}
	front := c.Def
	if c.frontDef != nil {
		front = c.frontDef
	}
	if front == nil || front.SplitType != carddb.SplitTransform || !front.Faces[1].Type.IsPermanent() && c.frontDef == nil {
		return false
	}
	if c.frontDef == nil {
		back := &compile.Card{Filename: front.Filename, Name: front.Faces[1].Name, SplitType: front.SplitType}
		back.Faces[0] = front.Faces[1]
		c.frontDef, c.Def = front, back
	} else {
		if !front.Faces[0].Type.IsPermanent() {
			return false
		}
		c.Def, c.frontDef = front, nil
	}
	g.timestamp++
	c.Timestamp = g.timestamp
	c.Transforms++
	return true
}

// setFaceDownState is CardFactoryUtil.setFaceDownState (:4129-4160) for a
// card SetState just turned face down: FaceDownPower$/FaceDownToughness$ set
// the printed values (read with the host, not the card, as the source),
// FaceDownSetType$ ("A & B") replaces the types. SetStateEffect.java:177-178
// never passes FaceDownKeyword$, so it is not read here either.
func (g *Game) setFaceDownState(a *Ability, host, c *Card) error {
	face := &c.Def.Faces[0]
	for _, e := range [...]struct {
		key string
		dst *string
	}{{"FaceDownPower", &face.Power}, {"FaceDownToughness", &face.Toughness}} {
		key, dst := e.key, e.dst
		raw, ok := a.Params.Param(key)
		if !ok {
			continue
		}
		v, ok := resolveNamedAmount(g, a.Amounts, host, raw)
		if !ok {
			return fmt.Errorf("engine: SetState: %s$ %q is not resolvable", key, raw)
		}
		*dst = strconv.Itoa(v)
	}
	if raw, ok := a.Params.Param("FaceDownSetType"); ok {
		var line cardtype.Line
		for _, word := range strings.Split(raw, " & ") {
			line = line.Union(cardtype.ParseToken(word))
		}
		face.Type = line
	}
	return nil
}

// specializeFaces maps the chosen colour to the face index of a Specialize
// card (Card.java:771-781, carddb.FaceSpecializeWhite..Green).
var specializeFaces = [...]struct {
	color mana.Colors
	face  int
}{
	{mana.White, carddb.FaceSpecializeWhite}, {mana.Blue, carddb.FaceSpecializeBlue},
	{mana.Black, carddb.FaceSpecializeBlack}, {mana.Red, carddb.FaceSpecializeRed},
	{mana.Green, carddb.FaceSpecializeGreen},
}

// specialize is Card.changeCardState("Specialize") (Card.java:771-781, ADR-0050)
// for the card id with the colour its source chose: the card's Def becomes the
// single face of that colour, under a new timestamp, and stays through every
// zone change. A card that has specialized already does nothing (canSpecialize,
// Card.java:6811). Java reads a null chosen colour without a guard
// (Card.java:771); here a colour that is not exactly one, a card with no such
// face, and a face-down, transformed or copied card are errors (GO-7).
func (g *Game) specialize(id CardID, colors mana.Colors) (bool, error) {
	c := g.Card(id)
	if c.specializedFrom != nil {
		return false, nil
	}
	if colors.Count() != 1 {
		return false, fmt.Errorf("engine: SetState: Specialize with %d chosen colors, want 1", colors.Count())
	}
	if c.IsFaceDown() || c.frontDef != nil || len(c.copies) > 0 {
		return false, fmt.Errorf("engine: SetState: Specialize of a face-down, transformed or copied card not resolvable yet")
	}
	for _, sf := range specializeFaces {
		if sf.color != colors {
			continue
		}
		face := c.Def.Faces[sf.face]
		if face.Name == "" {
			return false, fmt.Errorf("engine: SetState: %s has no %v Specialize face", c.Def.Name, colors)
		}
		def := &compile.Card{Filename: c.Def.Filename, Name: face.Name, SplitType: carddb.SplitSpecialize}
		def.Faces[0] = face
		c.specializedFrom, c.Def = c.Def, def
		g.timestamp++
		c.Timestamp = g.timestamp
		return true, nil
	}
	return false, fmt.Errorf("engine: SetState: Specialize with colors %v", colors)
}
