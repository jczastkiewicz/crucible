// Kicker (CR 702.33): an optional additional cost chosen as a spell is cast.
// Java records each paid kicker as an OptionalCost on the cast SpellAbility
// (Kicker1, Kicker2) and reads it back through Card.getKickerMagnitude,
// SpellAbility.isKicked, the `kicked` valid property, Count$Kicked and
// Condition$ Kicked.

//enginelint:allow id card game control mana zone ability unlesscost

package engine

import (
	"fmt"
	"strconv"
	"strings"

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

// keywordManaCost is the mana part of the keyword line k (K:<Name>:<cost>).
// A cost with a Tap, Untap, Mandatory or XMin token, or an X in its mana, is
// an error, never half-paid (GO-7); the named parts it carries come from
// keywordPartsText.
func keywordManaCost(c *Card, k keyword.Keyword) (mana.Cost, error) {
	args := k.Args()
	if len(args) == 0 {
		return mana.Cost{}, fmt.Errorf("engine: %q: %s %q not resolvable yet", c.Def.Name, k.Name, k.Details)
	}
	parsed := cost.Parse(args[0])
	if parsed.Tap || parsed.Untap || parsed.Mandatory || parsed.XMin != "" {
		return mana.Cost{}, fmt.Errorf("engine: %q: %s %q not resolvable yet", c.Def.Name, k.Name, k.Details)
	}
	if len(parsed.Mana) == 0 {
		return mana.Cost{}, nil
	}
	mc, err := mana.Parse(strings.Join(parsed.Mana, " "))
	if err != nil || mc.CountX() > 0 {
		return mana.Cost{}, fmt.Errorf("engine: %q: %s %q not resolvable yet", c.Def.Name, k.Name, args[0])
	}
	return mc, nil
}

// keywordPartsText is the named, non-mana parts of the keyword line k paid n
// times at once: GameActionUtil.addExtraKeywordCost (:675-681, :707-713,
// :725-731) adds the Cost to the spell's cost n times, which this writes as
// one part with its amount multiplied (tapXType<1/Horror> twice is
// tapXType<2/Horror>), so one payability check covers the shared resources.
// ok is false for a part whose amount is not a number (an X).
func keywordPartsText(k keyword.Keyword, n int) (text string, ok bool) {
	args := k.Args()
	if len(args) == 0 {
		return "", true
	}
	var parts []string
	for _, p := range cost.Parse(args[0]).Parts {
		if len(p.Fields) == 0 {
			parts = append(parts, p.Text)
			continue
		}
		amount, err := strconv.Atoi(p.Fields[0])
		if err != nil || amount < 0 {
			return "", false
		}
		fields := append([]string{strconv.Itoa(amount * n)}, p.Fields[1:]...)
		parts = append(parts, p.Name+"<"+strings.Join(fields, "/")+">")
	}
	return strings.Join(parts, " "), true
}

// keywordHasParts reports whether the keyword line k names a part past mana.
func keywordHasParts(k keyword.Keyword) bool {
	args := k.Args()
	return len(args) > 0 && len(cost.Parse(args[0]).Parts) > 0
}

// keywordPartsPayable reports whether pid can pay k's non-mana parts n times
// at once.
func (g *Game) keywordPartsPayable(pid PlayerID, c *Card, k keyword.Keyword, n int) bool {
	text, ok := keywordPartsText(k, n)
	if !ok {
		return false
	}
	uc, ok := parseUnlessCost(text)
	return ok && g.unlessPayable(pid, c.ID, uc)
}

// keywordAmountParts is the non-mana parts the chosen keyword amounts add to
// the cast, each keyword's parts once for all its repetitions, paid after the
// mana (payOptionalParts).
func keywordAmountParts(c *Card, amounts []keywordAmount) ([]unlessCost, error) {
	var out []unlessCost
	for _, a := range amounts {
		for _, line := range c.KeywordLines() {
			k := keyword.Parse(line)
			if k.Name != a.name || !keywordHasParts(k) {
				continue
			}
			text, ok := keywordPartsText(k, a.n)
			if !ok {
				return nil, fmt.Errorf("engine: %q: %s %q not resolvable yet", c.Def.Name, k.Name, k.Details)
			}
			uc, ok := parseUnlessCost(text)
			if !ok {
				return nil, fmt.Errorf("engine: %q: %s %q not resolvable yet", c.Def.Name, k.Name, k.Details)
			}
			out = append(out, uc)
			break
		}
	}
	return out, nil
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
			// Bounded by the mana pid can make and by the non-mana parts pid can
			// pay that many times at once.
			limit := keywordRepeatCap
			if per := each.Generic() + len(each.Shards()); per > 0 {
				limit = g.manaCapacity(pid) / per
			}
			if keywordHasParts(k) {
				for limit > 0 && !g.keywordPartsPayable(pid, c, k, limit) {
					limit--
				}
			}
			n = controller.ChooseNumber(g, pid, c.ID, 0, limit)
		}
		if n > 0 {
			out = append(out, keywordAmount{name: k.Name, n: n})
		}
	}
	return out, true
}

// keywordRepeatCap bounds a repeated keyword cost with no mana in it, which
// manaCapacity cannot bound; Java offers Integer.MAX_VALUE.
const keywordRepeatCap = 20

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
		if a.name == "Casualty" || a.name == "Conspire" {
			continue // paid by their own non-mana optional cost parts
		}
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
