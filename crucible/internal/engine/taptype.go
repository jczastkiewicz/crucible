// Tapping N untapped permanents of a type as an activation cost:
// tapXType<N/Type>, CR 602 trimmed to CostTapType.java's own literal-type
// shape -- Vault of the Archangel's own "Tap three untapped Creatures you
// control," a checkland-style "Tap an untapped Wizard you control," and the
// like. ActivateAbility/ActivateManaAbility's own ActivationShape.TapTypeN/
// TapTypeSpec (internal/cost) carry the count and the raw, unparsed type
// field; this file is where that field actually becomes a battlefield walk.
//
// Two of CostTapType.java's own real shapes are deliberately not built:
// "Creature+withTotalPowerGE<N>" (3 real corpus lines, "total power N or
// greater" rather than a card count) and
// "Creature.sharesCreatureTypeWith" (0 real corpus lines combined with a
// literal N) each need a different feasibility question than "N cards
// matching a spec," not merely a different spec -- tapTypeResolvable, below,
// refuses both explicitly rather than letting an unrecognized property
// silently match nothing (PORT-8/GO-7). "OriginalHost" (0 real corpus
// lines) -- "the permanent this ability was originally printed on," a
// concept this port's own card-copy machinery does not track -- is refused
// the identical way. Crew's own "tap N power of creatures to crew a
// Vehicle" reaches this same Cost part in Java (ability.isCrew()'s own
// branch, CostPartWithList#canPayListAtOnce), but this port has no Vehicle/
// Crew mechanic at all, so every real corpus line combining tapXType with a
// K:Crew keyword line resolves (or not) as an ordinary tapXType cost, never
// as crewing specifically.

package engine

import (
	"strings"

	"github.com/jczastkiewicz/crucible/internal/valid"
)

// tapTypeResolvable reports whether rawSpec (ActivationShape.TapTypeSpec) is
// a shape this port's own Matches (valid.go) can evaluate -- refusing the
// two CostTapType.java suffixes it has no property for
// (withTotalPowerGE/sharesCreatureTypeWith, 3 combined real corpus lines)
// and the one literal value with no card-count meaning at all
// (OriginalHost). A regression-toggle check found this guard is not, on its
// own, the only thing standing between a correct decline and a wrong tap
// here: internal/valid's own fail-safe contract ("a base it does not
// recognise simply matches nothing," valid.go's own doc comment, and
// empirically the identical fail-safe for an unrecognized Property) already
// makes Matches return false for every candidate against either suffix, so
// tapTypeCandidates' own count naturally falls short of TapTypeN and the
// caller declines anyway even with this guard disabled. Kept regardless,
// for the same reason every other effect in this port names its own
// unresolved params explicitly rather than trusting an implicit fail-safe
// three files away: a future change to Matches' own property dispatch could
// silently start matching one of these two shapes, and an explicit refusal
// here is what would catch that before it ever reached a real game.
func tapTypeResolvable(rawSpec string) bool {
	return rawSpec != "OriginalHost" &&
		!strings.Contains(rawSpec, "withTotalPowerGE") &&
		!strings.Contains(rawSpec, "sharesCreatureTypeWith")
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
	return candidates
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
	for _, id := range ids {
		c := g.Card(id)
		if c.Tapped {
			continue
		}
		c.Tapped = true
		g.checkTapsTriggers(controller, id, c.Controller(), false)
	}
}
