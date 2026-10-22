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

// keywordManaCost is the plain mana cost of the keyword line k
// (K:<Name>:<cost>). A cost that is not plain mana is an error, never
// half-paid (GO-7).
func keywordManaCost(c *Card, k keyword.Keyword) (mana.Cost, error) {
	args := k.Args()
	if len(args) == 0 || !cost.Parse(args[0]).IsPureMana() {
		return mana.Cost{}, fmt.Errorf("engine: %q: %s %q not resolvable yet", c.Def.Name, k.Name, k.Details)
	}
	mc, err := mana.Parse(args[0])
	if err != nil || mc.CountX() > 0 {
		return mana.Cost{}, fmt.Errorf("engine: %q: %s %q not resolvable yet", c.Def.Name, k.Name, args[0])
	}
	return mc, nil
}

// chooseKeywordAmounts is GameActionUtil.addExtraKeywordCost
// (GameActionUtil.java:617-734) for the keywords whose cost is mana: Multikicker,
// Squad and Replicate ask how many times to pay it
// (chooseNumberForKeywordCost), Offspring whether to (addKeywordCost). Only a
// positive amount is recorded. Java offers Integer.MAX_VALUE times; this port
// bounds the question by the mana pid can still make (manaCapacity), since
// more could never be paid. ok is false when a keyword's cost cannot be read.
func (g *Game) chooseKeywordAmounts(controller PlayerController, pid PlayerID, c *Card) (out []keywordAmount, ok bool) {
	for _, line := range c.KeywordLines() {
		k := keyword.Parse(line)
		switch k.Name {
		case "Multikicker", "Squad", "Replicate", "Offspring":
		default:
			continue
		}
		each, err := keywordManaCost(c, k)
		if err != nil {
			g.recordPendingError(err)
			return nil, false
		}
		n := 0
		if k.Name == "Offspring" {
			if controller.ConfirmPayCost(g, pid, cost.Parse(k.Args()[0]), c.ID) {
				n = 1
			}
		} else {
			per := each.Generic() + len(each.Shards())
			if per == 0 {
				per = 1
			}
			n = controller.ChooseNumber(g, pid, c.ID, 0, g.manaCapacity(pid)/per)
		}
		if n > 0 {
			out = append(out, keywordAmount{name: k.Name, n: n})
		}
	}
	return out, true
}

// manaCapacity is an upper bound on the mana pid could still make: what is in
// the pool plus one for each untapped land or permanent with a mana ability.
// It bounds a count the controller chooses; the payment is what enforces
// affordability.
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

// withKeywordAmounts is base plus each recorded keyword's mana cost, once per
// amount.
func withKeywordAmounts(c *Card, amounts []keywordAmount, base mana.Cost) mana.Cost {
	if len(amounts) == 0 || base.IsNoCost() {
		return base
	}
	generic, shards := base.Generic(), append([]mana.Shard(nil), base.Shards()...)
	for _, a := range amounts {
		for _, line := range c.KeywordLines() {
			k := keyword.Parse(line)
			if k.Name != a.name {
				continue
			}
			each, _ := keywordManaCost(c, k)
			for range a.n {
				generic += each.Generic()
				shards = append(shards, each.Shards()...)
			}
			break
		}
	}
	return mana.FromShards(shards, generic)
}
