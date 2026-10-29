// AlternateAdditionalCost (K:AlternateAdditionalCost:<cost>:<cost>...): "as an
// additional cost to cast this spell, <cost> or <cost>". Java offers one
// SpellAbility per alternative, each with that cost added to the spell's own,
// and drops the ones that cannot be played (GameActionUtil.
// getAdditionalCostSpell, GameActionUtil.java:570-591); the player casts one
// of them. Here the caster picks the alternative as the cast starts, its
// mana joins the cost and its other parts are paid as the spell's additional
// cost, as a MayPlay grant's RaiseCost$ is (castFromHand). An alternative
// whose parts parseUnlessCost does not read, or that cannot be paid now, is
// not offered; a card with none left is not cast, never cast free of the
// cost (GO-7). An alternative with non-mana parts on a cast that already
// pays another additional cost (Escape, Beam me up) is not offered either:
// paying two is not modelled.

package engine

//enginelint:allow id card game control keyword unlesscost castspell

import (
	"fmt"
	"strings"

	"github.com/jczastkiewicz/crucible/internal/keyword"
)

// alternateAdditionalCosts is c's AlternateAdditionalCost alternatives, in
// keyword order, nil when it has none.
func alternateAdditionalCosts(c *Card) []string {
	for _, line := range c.KeywordLines() {
		k := keyword.Parse(line)
		if k.Name == "AlternateAdditionalCost" && k.Details != "" {
			return strings.Split(k.Details, ":")
		}
	}
	return nil
}

// chooseAlternateAdditionalCost folds the alternative pid picks into opts.
// ok is false when the card has alternatives and none can be paid, or the
// controller's answer is out of range (a pending error).
func (g *Game) chooseAlternateAdditionalCost(controller PlayerController, pid PlayerID, card CardID, opts castOpts) (castOpts, bool) {
	alts := alternateAdditionalCosts(g.Card(card))
	if len(alts) == 0 {
		return opts, true
	}
	var labels []string
	var costs []unlessCost
	for _, text := range alts {
		uc, ok := parseUnlessCost(text)
		if !ok || !g.unlessPayable(pid, card, uc) {
			continue
		}
		if uc.hasParts() && opts.hasExtra {
			continue
		}
		labels = append(labels, "Cast "+g.Card(card).Def.Name+" paying "+text+" as its additional cost")
		costs = append(costs, uc)
	}
	pick := 0
	switch len(costs) {
	case 0:
		return opts, false
	case 1:
	default:
		pick = controller.ChooseOption(g, pid, card, labels)
		if pick < 0 || pick >= len(costs) {
			g.recordPendingError(fmt.Errorf("engine: card %d: additional cost %d is not one of %d", card, pick, len(costs)))
			return opts, false
		}
	}
	uc := costs[pick]
	if uc.hasMana {
		if opts.hasRaiseMana {
			opts.raiseMana = withExtraMana(opts.raiseMana, uc.mana)
		} else {
			opts.raiseMana, opts.hasRaiseMana = uc.mana, true
		}
	}
	if uc.hasParts() {
		opts.extra, opts.hasExtra = uc, true
	}
	return opts, true
}
