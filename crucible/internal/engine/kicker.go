// Kicker (CR 702.33): an optional additional cost chosen as a spell is cast.
// Java records each paid kicker as an OptionalCost on the cast SpellAbility
// (Kicker1, Kicker2) and reads it back through Card.getKickerMagnitude,
// SpellAbility.isKicked, the `kicked` valid property, Count$Kicked and
// Condition$ Kicked.

//enginelint:allow id card game control mana zone ability

package engine

import (
	"fmt"

	"github.com/jczastkiewicz/crucible/internal/carddb/compile"
	"github.com/jczastkiewicz/crucible/internal/cardtype"
	"github.com/jczastkiewicz/crucible/internal/cost"
	"github.com/jczastkiewicz/crucible/internal/keyword"
	"github.com/jczastkiewicz/crucible/internal/mana"
)

// kickerOption is one kicker cost: its script text (what ConfirmPayCost shows)
// and the mana it adds.
type kickerOption struct {
	text string
	mana mana.Cost
}

// kickerCosts is the plain mana costs of c's Kicker keyword: "Kicker:<cost>"
// is one, "Kicker:<cost>:<cost>" two. A Kicker with a non-mana cost (Sac<..>)
// is not offered, never half-paid (GO-7).
func kickerCosts(c *Card) []kickerOption {
	for _, line := range c.KeywordLines() {
		k := keyword.Parse(line)
		if k.Name != "Kicker" {
			continue
		}
		var out []kickerOption
		for _, text := range k.Args() {
			if parsed := cost.Parse(text); !parsed.IsPureMana() {
				return nil
			}
			mc, err := mana.Parse(text)
			if err != nil || mc.CountX() > 0 {
				return nil
			}
			out = append(out, kickerOption{text: text, mana: mc})
		}
		return out
	}
	return nil
}

// chooseKicker asks pid, for each kicker cost c has, whether to pay it
// (ConfirmPayCost, the same "pay this optional cost?" question an unless cost
// asks), and returns the bits chosen -- also for a free cast, which still pays
// the optional costs (CR 118.9d; PlaySpellAbility.chooseOptionalAdditionalCosts).
func (g *Game) chooseKicker(controller PlayerController, pid PlayerID, c *Card) uint8 {
	options := kickerCosts(c)
	if len(options) == 0 {
		return 0
	}
	var bits uint8
	for i, o := range options {
		if controller.ConfirmPayCost(g, pid, cost.Parse(o.text), c.ID) {
			bits |= kicker1 << i
		}
	}
	return bits
}

// withKicker is base plus the cost of every kicker in bits.
func withKicker(c *Card, bits uint8, base mana.Cost) mana.Cost {
	if bits == 0 || base.IsNoCost() {
		return base
	}
	generic, shards := base.Generic(), append([]mana.Shard(nil), base.Shards()...)
	for i, o := range kickerCosts(c) {
		if bits&(kicker1<<i) != 0 {
			generic += o.mana.Generic()
			shards = append(shards, o.mana.Shards()...)
		}
	}
	return mana.FromShards(shards, generic)
}

// multikickerCost is the plain mana cost of c's Multikicker keyword
// (K:Multikicker:<cost>). ok is false without the keyword; a cost that is not
// plain mana comes back as an error, never half-paid (GO-7).
func multikickerCost(c *Card) (m mana.Cost, ok bool, err error) {
	for _, line := range c.KeywordLines() {
		k := keyword.Parse(line)
		if k.Name != "Multikicker" {
			continue
		}
		args := k.Args()
		if len(args) == 0 || !cost.Parse(args[0]).IsPureMana() {
			return mana.Cost{}, false, fmt.Errorf("engine: %q: Multikicker %q not resolvable yet", c.Def.Name, k.Details)
		}
		mc, perr := mana.Parse(args[0])
		if perr != nil || mc.CountX() > 0 {
			return mana.Cost{}, false, fmt.Errorf("engine: %q: Multikicker %q not resolvable yet", c.Def.Name, args[0])
		}
		return mc, true, nil
	}
	return mana.Cost{}, false, nil
}

// chooseMultikicker asks pid how many times to pay c's Multikicker cost
// (PlayerController.chooseNumberForKeywordCost, GameActionUtil.java:667-685).
// Java offers up to Integer.MAX_VALUE; this port bounds the question by the
// mana pid can still make (manaCapacity), since more copies could never be
// paid. ok is false when the keyword's cost cannot be read.
func (g *Game) chooseMultikicker(controller PlayerController, pid PlayerID, c *Card) (n int, ok bool) {
	each, has, err := multikickerCost(c)
	if err != nil {
		g.recordPendingError(err)
		return 0, false
	}
	if !has {
		return 0, true
	}
	per := each.Generic() + len(each.Shards())
	if per == 0 {
		per = 1
	}
	return controller.ChooseNumber(g, pid, c.ID, 0, g.manaCapacity(pid)/per), true
}

// manaCapacity is an upper bound on the mana pid could still make: what is in
// the pool plus one for each untapped permanent they control that has a mana
// ability, as many as it could make at most counted once. It bounds a count
// the controller chooses; the payment is what enforces affordability.
func (g *Game) manaCapacity(pid PlayerID) int {
	n := g.Player(pid).ManaPool.Total()
	for _, id := range g.Zone(Battlefield, pid).Cards() {
		c := g.Card(id)
		if c.Tapped {
			continue
		}
		if c.Type().Has(cardtype.Land) {
			n++
			continue
		}
		for i := range c.abilityCount() {
			if a, _, found := c.abilityAt(i); found && a.Record == compile.Activated && a.Name == "Mana" {
				n++
				break
			}
		}
	}
	return n
}

// withMultikicker is base plus n copies of c's Multikicker cost.
func withMultikicker(c *Card, n int, base mana.Cost) mana.Cost {
	if n <= 0 || base.IsNoCost() {
		return base
	}
	each, _, _ := multikickerCost(c)
	generic, shards := base.Generic(), append([]mana.Shard(nil), base.Shards()...)
	for range n {
		generic += each.Generic()
		shards = append(shards, each.Shards()...)
	}
	return mana.FromShards(shards, generic)
}
