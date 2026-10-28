// Illusionary Mask: a creature cast face down by Play's CastFaceDown$, and the
// replacement ReplaceIlluMask$ attaches to it.
//
// Ported from forge-game/src/main/java/forge/game/ability/effects/PlayEffect.java
// (:321-324, :458-459, :538-575), CardFactoryUtil.abilityCastFaceDown
// (CardFactoryUtil.java:80-120) and CardProperty.java:2072-2128 (CanPayManaCost).
// ADR-0048.

package engine

//enginelint:allow game ability card id valid mana facedown triggerpack zone control

import (
	"fmt"

	"github.com/jczastkiewicz/crucible/internal/mana"
)

// turnFaceDownForCast makes the card in hand the face-down 2/2 its cast puts
// on the stack (CR 708.4: turned face down before it goes on the stack, so every
// cost and restriction sees only those characteristics; Card.java:7476-7478,
// CostAdjustment.java:50-52). castSpell turns it face up again if the cast fails.
func (g *Game) turnFaceDownForCast(id CardID) {
	c := g.Card(id)
	c.clearTextChange()
	if c.faceUpDef == nil {
		c.faceUpDef = c.Def
	}
	c.Def = faceDownDef(false)
}

// maskTurnUp is ReplaceIlluMask's three Updated replacements (PlayEffect.java:
// 538-575): a face-down creature Mask cast, about to assign or deal damage, be
// dealt damage or become tapped, turns face up first (SetState Mode$
// TurnFaceUp), then the event goes on. The Command-zone effect card Java makes
// has no other observable behaviour, so the flag on the creature stands for it
// (ADR-0048).
func (g *Game) maskTurnUp(controller PlayerController, id CardID) {
	c := g.Card(id)
	if !c.illuMask || !c.IsFaceDown() || !g.canBeTurnedFaceUp(id) {
		return
	}
	c.turnFaceUp()
	// CR 613.7f: a permanent turned face up gets a new timestamp (Card.java:891).
	g.timestamp++
	c.Timestamp = g.timestamp
	g.faceChangeReplaced(controller, "TurnFaceUp", id)
	g.checkTurnedFaceUpTriggers(controller, id, nil)
}

// canPayManaCostWith is Valid$'s CanPayManaCost (CardProperty.java:2072-2128):
// whether some of the mana the activation spent pays c's mana cost, one unit per
// shard, an X shard ignored. A colored shard takes mana of one of its colors, a
// generic shard (and a {2/C} half) any mana; Java tries the colored match and
// the generic match for the same unit one after the other, and so does this.
// Snow shards and Phyrexian life payments are not modelled: a cost with one is
// an error, not a guess (GO-7).
func canPayManaCostWith(c *Card, paid paidLists) (bool, error) {
	cost := c.manaCost()
	var shards []mana.Shard
	for _, s := range cost.Shards() {
		switch {
		case s.IsX() && s == mana.ShardX:
			continue
		case s == mana.ShardSnow || s.IsPhyrexian():
			return false, fmt.Errorf("engine: CanPayManaCost: %s's cost has a %s shard, not resolvable yet", c.Def.Name, s)
		}
		shards = append(shards, s)
	}
	for range cost.Generic() {
		shards = append(shards, mana.ShardGeneric)
	}
	// units: 0-4 white to green, 5 colorless.
	var units []int
	for color, n := range paid.colors {
		for range n {
			units = append(units, color)
		}
	}
	for range paid.colorless {
		units = append(units, 5)
	}
	var check func(i int, left []int) bool
	check = func(i int, left []int) bool {
		if i >= len(shards) {
			return true
		}
		s := shards[i]
		for k, u := range left {
			rest := append(append([]int(nil), left[:k]...), left[k+1:]...)
			colored := u < 5 && s.Colors().Has(mana.Colors(1)<<u) && s.Colors() != 0
			colorless := u == 5 && (s == mana.ShardC || s == mana.ShardCW || s == mana.ShardCU || s == mana.ShardCB || s == mana.ShardCR || s == mana.ShardCG)
			if (colored || colorless) && check(i+1, rest) {
				return true
			}
			if s.IsGeneric() || s.IsOr2Generic() {
				next := rest
				ahead := shards
				if s.IsOr2Generic() {
					// "Handle 2 generic mana": the second half is a generic shard.
					shards = append(append(append([]mana.Shard(nil), shards[:i+1]...), mana.ShardGeneric), shards[i+1:]...)
				}
				ok := check(i+1, next)
				shards = ahead
				if ok {
					return true
				}
			}
		}
		return false
	}
	return check(0, units), nil
}
