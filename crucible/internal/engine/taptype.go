// Tapping N untapped permanents of a type as an activation cost:
// tapXType<N/Type>, CR 602 trimmed to CostTapType.java's own literal-type
// shape -- Vault of the Archangel's own "Tap three untapped Creatures you
// control," a checkland-style "Tap an untapped Wizard you control," and the
// like. ActivateAbility/ActivateManaAbility's own ActivationShape.TapTypeN/
// TapTypeSpec (internal/cost) carry the count and the raw, unparsed type
// field; this file is where that field actually becomes a battlefield walk.
//
// "Creature.sharesCreatureTypeWith" (Weight of Conscience) keeps the
// candidates that share a creature type with another one and checks the pick
// (tapSharedTypeOK). "Creature+withTotalPowerGE<N>" is Crew's shape, read by
// internal/cost into TapTypeTotalPower. "OriginalHost" (Fishing Pole) is the
// one card tapped: the card whose static granted the ability, or the ability's
// own card for a printed line (SpellAbility.getOriginalHost, CostTapType.java:
// 150-151, HumanCostDecision.java:1374-1379). An unless-cost still refuses it
// (tapTypeResolvable): no corpus line pays one.

package engine

import (
	"slices"
	"strings"

	"github.com/jczastkiewicz/crucible/internal/valid"
)

// tapTypeResolvable reports whether rawSpec (ActivationShape.TapTypeSpec) is
// a shape this port evaluates: not OriginalHost (see the file comment), and
// not a withTotalPowerGE spec that reached the plain count path. Matches
// would match nothing for either, so the check only names the refusal
// explicitly (GO-7).
func tapTypeResolvable(rawSpec string) bool {
	return rawSpec != "OriginalHost" &&
		!strings.Contains(rawSpec, "withTotalPowerGE")
}

// tapTypeActivatable is tapTypeResolvable for an activation cost, which also
// reads OriginalHost.
func tapTypeActivatable(rawSpec string) bool {
	return rawSpec == "OriginalHost" || tapTypeResolvable(rawSpec)
}

// originalHostCandidates is the tapXType<N/OriginalHost> candidate list: the
// ability's original host (origin, NoCard for a printed line -> source) when
// it can be tapped (CostTapType.canPay: getOriginalHost().canTap()).
func originalHostCandidates(g *Game, source, origin CardID) []CardID {
	host := origin
	if host == NoCard {
		host = source
	}
	if c := g.Card(host); c.Zone != Battlefield || c.Tapped {
		return nil
	}
	return []CardID{host}
}

// sharedCreatureTypeSuffix is CostTapType's ".sharesCreatureTypeWith"
// (Weight of Conscience): the tapped creatures share a creature type.
const sharedCreatureTypeSuffix = ".sharesCreatureTypeWith"

// chooseTapType is the payer's pick of n of candidates for a tapXType part.
// Under .sharesCreatureTypeWith each creature tapped shares a creature type
// with every one tapped before it (HumanCostDecision.java:1420-1434); a pick
// that breaks that is replaced by the greedy one from the first candidate,
// which tapTypeCandidates' filter makes possible for the corpus's N of 2.
func chooseTapType(g *Game, controller PlayerController, pid PlayerID, candidates []CardID, n int, rawSpec string) []CardID {
	chosen := controller.ChoosePermanentsToTap(g, pid, candidates, n)
	if !strings.Contains(rawSpec, sharedCreatureTypeSuffix) || sharesCreatureTypeAll(g, chosen) && len(chosen) == n && isSubset(chosen, candidates) {
		return chosen
	}
	var greedy []CardID
	for _, id := range candidates {
		if len(greedy) < n && sharesCreatureTypeAll(g, append(slices.Clone(greedy), id)) {
			greedy = append(greedy, id)
		}
	}
	return greedy
}

// sharesCreatureTypeAll reports whether every card in ids shares a creature
// type with every other.
func sharesCreatureTypeAll(g *Game, ids []CardID) bool {
	for i, id := range ids {
		for _, prev := range ids[:i] {
			if !g.sharesCreatureType(g.Card(prev), g.Card(id)) {
				return false
			}
		}
	}
	return true
}

// tapTypeCandidates finds every one of controller's own untapped battlefield
// permanents rawSpec matches -- CostTapType.java's own canPay/getMaxAmountX,
// ported: a semicolon-separated OR list becomes valid.Parse's own
// comma-separated one (Cost syntax's own reason for choosing ";" over ","
// -- a literal "," can appear in the Cost part's own trailing description
// field, the third body field this port never reads at all); CAN_TAP
// becomes a plain !Tapped read, since this port tracks no CantTap-shaped
// static ability to consult the way Java's own CardPredicates.CAN_TAP does;
// excludeSelf mirrors CostTapType's own canTapSource = !costHasTapSource --
// a cost that also taps its own source through a separate plain T token can
// never count that identical permanent toward tapXType's own total too, even
// when the type spec itself does not say ".Other".
func tapTypeCandidates(g *Game, pid PlayerID, source CardID, excludeSelf bool, rawSpec string) []CardID {
	sameType := strings.Contains(rawSpec, sharedCreatureTypeSuffix)
	rawSpec = strings.Replace(rawSpec, sharedCreatureTypeSuffix, "", 1)
	spec := valid.Parse(strings.ReplaceAll(rawSpec, ";", ","))
	var candidates []CardID
	for _, cid := range g.Zone(Battlefield, pid).Cards() {
		if excludeSelf && cid == source {
			continue
		}
		c := g.Card(cid)
		if c.Tapped {
			continue
		}
		if !Matches(g, c, spec, pid, source) {
			continue
		}
		candidates = append(candidates, cid)
	}
	if !sameType {
		return candidates
	}
	// CostTapType.canPay (:174-181) and the human pick (HumanCostDecision.
	// java:1407-1418): only a creature sharing a creature type with another
	// candidate can be tapped.
	var shared []CardID
	for _, id := range candidates {
		for _, other := range candidates {
			if other != id && g.sharesCreatureType(g.Card(id), g.Card(other)) {
				shared = append(shared, id)
				break
			}
		}
	}
	return shared
}

// tapChosenPermanents actually taps each of ids -- CostTapType.java's own
// doListPayment, minus TriggerType.TapAll (2 real corpus T: lines, not worth
// a dedicated batched-tap trigger mode). Each tapped card still fires the
// ordinary "becomes tapped" trigger (checkTapsTriggers, trigger.go), the
// identical per-card firing a Tap-self cost already gets -- a permanent
// already tapped by the time this runs (chosen twice, or tapped by an
// earlier part of the same cost) is skipped rather than firing a trigger
// for a tap that does not actually happen.
func tapChosenPermanents(g *Game, controller PlayerController, ids []CardID) {
	var tapped []CardID
	for _, id := range ids {
		c := g.Card(id)
		if c.Tapped {
			continue
		}
		c.Tapped = true
		g.checkTapsTriggers(controller, id, c.Controller(), false)
		tapped = append(tapped, id)
	}
	g.checkTapAllTriggers(controller, tapped)
}

// totalPower is the sum of the power of ids (a power this port cannot resolve
// counts as 0, never as a guess).
func totalPower(g *Game, ids []CardID) int {
	total := 0
	for _, id := range ids {
		if p, ok := g.Card(id).Power(); ok && p > 0 {
			total += p
		}
	}
	return total
}

// isSubset reports whether every id of sub is in set, and sub has no repeat.
func isSubset(sub, set []CardID) bool {
	for i, id := range sub {
		if !containsCard(set, id) || containsCard(sub[:i], id) {
			return false
		}
	}
	return true
}
