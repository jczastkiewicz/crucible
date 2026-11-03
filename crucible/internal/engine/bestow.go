// Bestow (CR 702.103): "If you cast this card for its bestow cost, it's an
// Aura spell with enchant creature. It becomes a creature again if it's not
// attached to a creature." The cast is one more keyword cast of the card
// (altkeywords.go) whose spell is an Aura spell: animateBestow makes the card
// an Aura with Enchant creature before the cast reads its type, so castAura
// targets, pays and pushes it; the change stays as the Aura enters (a move to
// the battlefield keeps the object, GameAction.java:171-172) and ends as it
// is unattached, as its target is gone on resolution (it then resolves as a
// creature spell) or as it leaves the stack for any other zone.
//
// Ported from forge-game/src/main/java/forge/game/card/CardFactoryUtil.java
// (the Bestow keyword branch, :2653-2688), card/Card.java's animateBestow and
// unanimateBestow (:6880-6911) and unattachFromEntity (:4003-4004),
// zone/MagicStack.java's fizzle branch (:692-700) and
// ability/effects/CounterEffect.java:260-261.

package engine

//enginelint:allow id zone card game animate typemod ability parts

import (
	"github.com/jczastkiewicz/crucible/internal/cardtype"
)

// animateBestow is Card.animateBestow: under a fresh timestamp the card loses
// the Creature type and its enchantment subtypes, gains the Aura subtype
// (RemoveType.EnchantmentTypes, then Aura) and Enchant:Creature, in whatever
// zone it is. A second call does nothing while the first holds. ok is false
// when the game's type vocabulary cannot tell the enchantment subtypes apart.
func (g *Game) animateBestow(id CardID) bool {
	c := g.Card(id)
	if c.bestowTS != 0 {
		return true
	}
	drop, ok := subtypeCategoryDrop(g, false, false, false, true)
	if !ok {
		return false
	}
	g.timestamp++
	c.bestowTS = g.timestamp
	g.addAnimate(animateRecord{
		Card: id, Timestamp: c.bestowTS, Permanent: true,
		Types: TypeEffect{
			AddTypes: cardtype.ParseToken("Aura"), RemoveTypes: cardtype.ParseToken("Creature"),
			DropSubtype: drop,
		},
		AddKeywords: []string{"Enchant:Creature"},
	})
	g.refreshLayers()
	return true
}

// unanimateBestow is Card.unanimateBestow: the bestow change ends and the
// card is the printed creature again. Nothing happens to a card that is not
// bestowed.
func (g *Game) unanimateBestow(id CardID) {
	if g.dropBestow(id) {
		g.refreshLayers()
	}
}

// dropBestow ends id's bestow change without rebuilding the layers, for a
// zone change, which rebuilds them itself. It reports whether there was one.
func (g *Game) dropBestow(id CardID) bool {
	c := g.Card(id)
	if c.bestowTS == 0 {
		return false
	}
	kept := g.animates[:0]
	for _, r := range g.animates {
		if r.Card != id || r.Timestamp != c.bestowTS {
			kept = append(kept, r)
		}
	}
	g.animates = kept
	c.bestowTS = 0
	return true
}

// Bestowed is Card.isBestowed: c is an Aura through its bestow cast.
func (c *Card) Bestowed() bool { return c.bestowTS != 0 }

// bestowFizzle is MagicStack.java:692-700 (CR 702.103e): a bestowed Aura
// spell whose target is illegal is not countered; the bestow change ends and
// it resolves as the creature spell it is. It reports whether a was such a
// spell, made the creature spell in place.
func (g *Game) bestowFizzle(a *Ability) bool {
	if !a.spell || a.API != APIAttach || !g.Card(a.Source).Bestowed() {
		return false
	}
	g.unanimateBestow(a.Source)
	a.API, a.Target, a.Targets = APIPermanentCreature, NoCard, nil
	return true
}
