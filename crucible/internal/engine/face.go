// Casting a card as its second face (ADR-0052): Adventure, Omen, Modal,
// Vengeance and the halves of a Split card that is not a Room (a Room's
// doors are room.go). The face is cast as a one-face view of the printed card
// (doorView) while the spell is on the stack, as Card.setSplitStateToPlayAbility
// (Card.java:7484-7504) does, so cost, type, timing and abilities all read it.

package engine

import (
	"strings"

	"github.com/jczastkiewicz/crucible/internal/carddb"
	"github.com/jczastkiewicz/crucible/internal/carddb/compile"
	"github.com/jczastkiewicz/crucible/internal/cardtype"
	"github.com/jczastkiewicz/crucible/internal/expr"
	"github.com/jczastkiewicz/crucible/internal/keyword"
	"github.com/jczastkiewicz/crucible/internal/mana"
)

// faceCastable reports whether def has a second face that is cast or played
// on its own (CardSplitType's SecondaryFace states; Prepare is prepare.go's).
func faceCastable(def *compile.Card) bool {
	if def == nil || def.Faces[carddb.FaceAlternate].Name == "" {
		return false
	}
	switch def.SplitType {
	case carddb.SplitSplit:
		return !isRoomDef(def)
	case carddb.SplitAdventure, carddb.SplitOmen, carddb.SplitModal, carddb.SplitVengeance:
		return true
	case carddb.SplitTransform:
		_, ok := disturbCost(def)
		return ok
	}
	return false
}

// disturbCost is the K:Disturb:<cost> of def's front face, the cost of casting
// its back face from the graveyard (CardFactoryUtil.java:2780-2810).
func disturbCost(def *compile.Card) (mana.Cost, bool) {
	for _, line := range def.Faces[carddb.FacePrimary].Keywords {
		k := keyword.Parse(line)
		if k.Name != "Disturb" || k.Details == "" {
			continue
		}
		mc, err := mana.Parse(k.Details)
		if err == nil && mc.CountX() == 0 {
			return mc, true
		}
	}
	return mana.Cost{}, false
}

// castAsFace makes c the spell of its second face; undoCastAsFace reverses it
// when the cast is declined.
func (c *Card) castAsFace() {
	c.faceDef = c.Def
	c.Def = doorView(c.faceDef, carddb.FaceAlternate)
	c.castDoor = carddb.FaceAlternate
}

func (c *Card) undoCastAsFace() { c.leaveFace() }

// leaveFace restores the printed card as c leaves the stack or the battlefield.
func (c *Card) leaveFace() {
	if c.faceDef == nil {
		return
	}
	c.Def = c.faceDef
	c.faceDef = nil
	c.castDoor = DoorLeft
}

// castFace reports the printed split type of a spell cast as its second face.
func (c *Card) castFace() (carddb.SplitType, bool) {
	if c.faceDef == nil || c.Zone != Stack {
		return carddb.SplitNone, false
	}
	return c.faceDef.SplitType, true
}

// CastFace casts card as its second face (Adventure, Omen, Modal, Vengeance or a
// Split half) with CastSpell's rules for that face's own cost, type and timing.
func (g *Game) CastFace(pid PlayerID, card CardID, controller PlayerController) bool {
	c := g.Card(card)
	if c.faceDef != nil || !faceCastable(c.Def) {
		return false
	}
	if faceGraveyardOnly(c.Def) && c.Zone != Graveyard {
		return false
	}
	c.castAsFace()
	if g.castFromHand(pid, card, DoorLeft, controller) {
		return true
	}
	if c.Zone != Stack {
		c.undoCastAsFace()
	}
	return false
}

// faceGraveyardOnly reports whether def'"'"'s second face is cast only from the
// graveyard: a Vengeance half or one with K:Aftermath
// (SpellAbilityRestriction.java:359-361).
func faceGraveyardOnly(def *compile.Card) bool {
	if def.SplitType == carddb.SplitVengeance {
		return true
	}
	for _, line := range def.Faces[carddb.FaceAlternate].Keywords {
		if keyword.Parse(line).Name == "Aftermath" {
			return true
		}
	}
	return false
}

// PlayLandFace plays a Modal card's land back face (LandAbility.java:56-69).
func (g *Game) PlayLandFace(pid PlayerID, card CardID, controller PlayerController) bool {
	c := g.Card(card)
	if c.faceDef != nil || !faceCastable(c.Def) || c.Def.SplitType != carddb.SplitModal ||
		!c.Def.Faces[carddb.FaceAlternate].Type.Has(cardtype.Land) {
		return false
	}
	c.castAsFace()
	if g.PlayLand(pid, card, controller) && c.Zone == Battlefield {
		return true
	}
	if c.Zone != Battlefield {
		c.undoCastAsFace()
	}
	return false
}

// faceZoneCost is the mana cost of casting c from a zone other than the hand
// because of its face: an Aftermath second face from its owner's graveyard
// (SpellAbilityRestriction.java:359-361, CardFactoryUtil.java:2617-2623), and
// an Adventure card's creature from exile while it is on an adventure
// (PlayerZone.java:64, Card.isOnAdventure).
func (g *Game) faceZoneCost(pid PlayerID, c *Card) (mana.Cost, bool) {
	if c.Owner != pid || c.Def == nil {
		return mana.Cost{}, false
	}
	if c.faceDef != nil && c.Zone == Graveyard && c.faceDef.SplitType == carddb.SplitTransform {
		// Disturb: the back face, cast transformed for the disturb cost.
		return disturbCost(c.faceDef)
	}
	if c.manaCost().IsNoCost() {
		return mana.Cost{}, false
	}
	switch {
	case c.faceDef != nil && c.Zone == Graveyard:
		if c.faceDef.SplitType == carddb.SplitVengeance {
			return c.manaCost(), true
		}
		for _, line := range c.KeywordLines() {
			if keyword.Parse(line).Name == "Aftermath" {
				return c.manaCost(), true
			}
		}
	case c.faceDef == nil && c.Zone == Exile && c.onAdventure:
		return c.manaCost(), true
	}
	return mana.Cost{}, false
}

// resolvedFaceSpell is where a face spell goes as it leaves the stack, in
// moveResolvedSpellToGraveyard (a resolved Adventure spell, not a countered
// one, CounterEffect.java:263 carries no Fizzle key) and moveToGraveyard (an
// Omen spell on any departure, CardFactoryUtil.java:4102-4116): Adventure
// exiles the card on an adventure, Omen shuffles it into its owner's library.
// Reports whether it took the card.
func (g *Game) resolvedFaceSpell(controller PlayerController, id CardID, resolved bool) bool {
	c := g.Card(id)
	split, ok := c.castFace()
	if !ok || c.IsCopiedSpell {
		return false
	}
	switch {
	case split == carddb.SplitAdventure && resolved:
		g.Move(id, Exile, c.Owner)
		g.Card(id).onAdventure = true
		return true
	case split == carddb.SplitOmen:
		owner := c.Owner
		g.MoveToLibraryTop(id, owner)
		g.shuffleLibrary(controller, owner, nil)
		return true
	}
	return false
}

// fuseView is the spell a Split card with Fuse is when both halves are cast
// (AbilityFactory.buildFusedAbility, :383-415): the combined name, cost, types
// and colors (CardFactory.java:301-313), the left half's spell with the right
// half's spell as the last link of its sub-ability chain, and both halves'
// amounts. ok is false for a card that is not a Fuse Split card of one
// A:SP$ line per half with no Cost$ of its own (this port pays the combined
// mana only).
func fuseView(printed *compile.Card) (*compile.Card, bool) {
	left, right := printed.Faces[carddb.FacePrimary], printed.Faces[carddb.FaceAlternate]
	if printed.SplitType != carddb.SplitSplit || isRoomDef(printed) || right.Name == "" {
		return nil, false
	}
	fuse := false
	for _, line := range left.Keywords {
		fuse = fuse || keyword.Parse(line).Name == "Fuse"
	}
	if !fuse {
		return nil, false
	}
	leftSpell, leftAt := onlySpell(left)
	rightSpell, _ := onlySpell(right)
	if leftSpell == nil || rightSpell == nil {
		return nil, false
	}
	for _, sp := range []*compile.Ability{leftSpell, rightSpell} {
		if _, has := sp.Param("Cost"); has {
			return nil, false
		}
	}
	tail := *rightSpell
	tail.Record = compile.SubAbility
	v := &compile.Card{Filename: printed.Filename, Name: left.Name + " // " + right.Name}
	v.Faces[0] = left
	v.Faces[0].Name = v.Name
	v.Faces[0].Type = left.Type.Union(right.Type)
	shards := append(append([]mana.Shard(nil), left.ManaCost.Shards()...), right.ManaCost.Shards()...)
	v.Faces[0].ManaCost = mana.FromShards(shards, left.ManaCost.Generic()+right.ManaCost.Generic())
	if left.HasColors || right.HasColors {
		v.Faces[0].Colors = faceColors(left) | faceColors(right)
		v.Faces[0].HasColors = true
	}
	v.Faces[0].Abilities = append([]*compile.Ability(nil), left.Abilities...)
	v.Faces[0].Abilities[leftAt] = chainWith(leftSpell, &tail)
	amounts := map[string]expr.Amount{}
	for k, a := range right.Amounts {
		amounts[k] = a
	}
	for k, a := range left.Amounts {
		amounts[k] = a
	}
	v.Faces[0].Amounts = amounts
	return v, true
}

// onlySpell is the one A:SP$ line of f and its index, nil when there is not
// exactly one.
func onlySpell(f compile.Face) (*compile.Ability, int) {
	var found *compile.Ability
	at := -1
	for i, ab := range f.Abilities {
		if ab.Record != compile.Spell {
			continue
		}
		if found != nil {
			return nil, -1
		}
		found, at = ab, i
	}
	return found, at
}

// chainWith is a copy of a whose sub-ability chain ends in tail
// (SpellAbility.appendSubAbility), the shared AST left untouched.
func chainWith(a, tail *compile.Ability) *compile.Ability {
	cp := *a
	cp.Subs = append([]compile.SubRef(nil), a.Subs...)
	for i := range cp.Subs {
		if strings.EqualFold(cp.Subs[i].Key, "SubAbility") {
			cp.Subs[i].Ability = chainWith(cp.Subs[i].Ability, tail)
			return &cp
		}
	}
	cp.Subs = append(cp.Subs, compile.SubRef{Key: "SubAbility", Ability: tail})
	return &cp
}

// CastFused casts a Fuse Split card's two halves together from the hand.
func (g *Game) CastFused(pid PlayerID, card CardID, controller PlayerController) bool {
	c := g.Card(card)
	if c.faceDef != nil || c.Zone != Hand {
		return false
	}
	view, ok := fuseView(c.Def)
	if !ok {
		return false
	}
	c.faceDef, c.Def, c.castDoor = c.Def, view, DoorLeft
	if g.castFromHand(pid, card, DoorLeft, controller) {
		return true
	}
	if c.Zone != Stack {
		c.undoCastAsFace()
	}
	return false
}

// OnAdventure reports whether c is an Adventure card in exile that may be cast as
// its creature (Card.isOnAdventure).
func (c *Card) OnAdventure() bool { return c.onAdventure && c.Zone == Exile }

// SetOnAdventure marks an exiled Adventure card as on an adventure, for a
// fixture's `OnAdventure` annotation (GameState.java:1366).
func (c *Card) SetOnAdventure(on bool) { c.onAdventure = on }
