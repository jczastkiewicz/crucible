// Kicker (CR 702.33): an optional additional cost chosen as a spell is cast.
// Java records each paid kicker as an OptionalCost on the cast SpellAbility
// (Kicker1, Kicker2) and reads it back through Card.getKickerMagnitude,
// SpellAbility.isKicked, the `kicked` valid property, Count$Kicked and
// Condition$ Kicked.

//enginelint:allow id card game control mana

package engine

import (
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
