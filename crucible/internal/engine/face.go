// Casting a card as its second face (ADR-0052): Adventure, Omen, Modal,
// Vengeance and the halves of a Split card that is not a Room (a Room's
// doors are room.go). The face is cast as a one-face view of the printed card
// (doorView) while the spell is on the stack, as Card.setSplitStateToPlayAbility
// (Card.java:7484-7504) does, so cost, type, timing and abilities all read it.

package engine

import (
	"github.com/jczastkiewicz/crucible/internal/carddb"
	"github.com/jczastkiewicz/crucible/internal/carddb/compile"
	"github.com/jczastkiewicz/crucible/internal/cardtype"
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
	}
	return false
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
	c.castAsFace()
	if g.castFromHand(pid, card, DoorLeft, controller) {
		return true
	}
	if c.Zone != Stack {
		c.undoCastAsFace()
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
	if c.Owner != pid || c.Def == nil || c.manaCost().IsNoCost() {
		return mana.Cost{}, false
	}
	switch {
	case c.faceDef != nil && c.Zone == Graveyard:
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
