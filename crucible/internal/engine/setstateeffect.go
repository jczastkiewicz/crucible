package engine

import (
	"fmt"

	"github.com/jczastkiewicz/crucible/internal/carddb"
	"github.com/jczastkiewicz/crucible/internal/carddb/compile"
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
// face up (CR 708.8). Optional$ asks first; RememberChanged$ remembers each
// changed card. Flip, TurnFaceDown and Specialize are not resolved.
type setStateEffect struct{}

func (setStateEffect) Resolve(g *Game, a *Ability, controller PlayerController) error {
	if err := rejectParams(a, "SetState", "Condition", "RevealFirst", "ValidNewFace", "NewState",
		"FaceDownPower", "FaceDownToughness", "FaceDownSetType", "ETB"); err != nil {
		return err
	}
	source := g.Card(a.Source)
	if !subAbilityConditionMet(g, source, a.Amounts, a.Params) {
		return nil
	}
	mode, _ := a.Params.Param("Mode")
	if mode != "Transform" && mode != "TurnFaceUp" {
		return fmt.Errorf("engine: SetState: Mode$ %q not resolvable yet", mode)
	}
	if mode == "Transform" && (battlefieldStaticMode(g, "CantTransform") || battlefieldReplacementEvent(g, "Transform")) {
		return fmt.Errorf("engine: SetState: CantTransform statics or Transform replacements not resolvable yet")
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
		min, err := optionalAmount(g, a, "SetState", "MinAmount", amount)
		if err != nil {
			return err
		}
		if !hasParam(a, "Mandatory") {
			min = 0
		}
		if amount > len(choices) {
			amount = len(choices)
		}
		if min > amount {
			min = amount
		}
		cards = controller.ChooseCardsForEffect(g, a.Controller, a.Source, choices, min, amount)
		if err := checkChoice(cards, choices, min, amount); err != nil {
			return fmt.Errorf("engine: SetState: %w", err)
		}
	} else {
		var err error
		if cards, err = targetedOrDefinedCards(source, a.Params, a.refs()); err != nil {
			return fmt.Errorf("engine: SetState: %w", err)
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
		if mode == "Transform" {
			changed = g.transform(id)
		} else if c.IsFaceDown() {
			if c.faceUpDef.Faces[0].Type.IsPermanent() {
				c.turnFaceUp()
				changed = true
			}
		}
		if changed && hasParam(a, "RememberChanged") {
			source.Memory.Remember(CardEntity(id))
		}
	}
	return nil
}

// transform is Card.changeCardState("Transform"): a transforming
// double-faced permanent that is face up and whose other face is a
// permanent turns to that face, under a new timestamp.
func (g *Game) transform(id CardID) bool {
	c := g.Card(id)
	if c.IsFaceDown() {
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
