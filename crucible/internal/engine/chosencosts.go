// The chosen-card parts of an activation cost: Sac<N/Type>, Exile<N/Type>,
// ExileFromGrave<N/Type> and Discard<N/Type>, where the payer picks N cards
// among those matching a type (CostSacrifice, CostExile, CostDiscard past
// their CARDNAME shapes). The picks are made before anything is paid, so a
// refused or illegal one costs nothing (CR 601.2h's ordering is free), then
// committed after the mana is.

//enginelint:allow id zone card game valid ability control discardeffect sacrificeeffect exile taptype exilefromgrave

package engine

import (
	"strings"

	"github.com/jczastkiewicz/crucible/internal/cost"
	"github.com/jczastkiewicz/crucible/internal/valid"
)

// chosenCosts is the cards picked for each chosen-card part.
type chosenCosts struct {
	sac, exile, exileGrave, discard []CardID
}

// costCandidates is the cards of zone pid owns or controls that spec (a Cost
// type: OR alternatives separated by ";") matches from source's point of view,
// source itself left out of a hand pick.
func (g *Game) costCandidates(pid PlayerID, source CardID, zone ZoneType, spec string) []CardID {
	parsed := valid.Parse(strings.ReplaceAll(spec, ";", ","))
	var out []CardID
	for _, id := range g.Zone(zone, pid).Cards() {
		c := g.Card(id)
		if zone == Battlefield && c.IsPhasedOut() || zone == Hand && id == source {
			continue
		}
		if Matches(g, c, parsed, pid, source) {
			out = append(out, id)
		}
	}
	return out
}

// chooseCostCards checks every chosen-card part of shape can be paid and has the
// controller pick its cards. false when one cannot, or when the pick is not N
// distinct candidates, or when two parts picked the same card.
func (g *Game) chooseCostCards(controller PlayerController, pid PlayerID, source CardID, shape cost.ActivationShape) (chosenCosts, bool) {
	var chosen chosenCosts
	pick := func(n int, spec string, zone ZoneType, choose func([]CardID) []CardID) ([]CardID, bool) {
		if n == 0 {
			return nil, true
		}
		candidates := g.costCandidates(pid, source, zone, spec)
		if len(candidates) < n {
			return nil, false
		}
		picked := choose(candidates)
		if len(picked) != n || !isSubset(picked, candidates) || hasDuplicate(picked) {
			return nil, false
		}
		return picked, true
	}
	var ok bool
	if chosen.sac, ok = pick(shape.SacTypeN, shape.SacTypeSpec, Battlefield, func(c []CardID) []CardID {
		return controller.ChoosePermanentsToSacrifice(g, pid, c, shape.SacTypeN)
	}); !ok {
		return chosenCosts{}, false
	}
	if chosen.exile, ok = pick(shape.ExileTypeN, shape.ExileTypeSpec, Battlefield, func(c []CardID) []CardID {
		return controller.ChooseCardsForEffect(g, pid, source, c, shape.ExileTypeN, shape.ExileTypeN)
	}); !ok {
		return chosenCosts{}, false
	}
	if chosen.exileGrave, ok = pick(shape.ExileGraveN, shape.ExileGraveSpec, Graveyard, func(c []CardID) []CardID {
		return controller.ChooseCardsForEffect(g, pid, source, c, shape.ExileGraveN, shape.ExileGraveN)
	}); !ok {
		return chosenCosts{}, false
	}
	if chosen.discard, ok = pick(shape.DiscardTypeN, shape.DiscardTypeSpec, Hand, func(c []CardID) []CardID {
		return controller.ChooseCardsToDiscard(g, pid, c, shape.DiscardTypeN)
	}); !ok {
		return chosenCosts{}, false
	}
	return chosen, !overlaps(chosen.sac, chosen.exile)
}

// payCostCards commits the picks and records them on a, the ability they paid
// for.
func (g *Game) payCostCards(controller PlayerController, pid PlayerID, chosen chosenCosts, a *Ability) {
	if len(chosen.sac) > 0 {
		sacrificeCards(g, controller, a, chosen.sac)
		a.paid.sacrificed = append(a.paid.sacrificed, chosen.sac...)
	}
	if len(chosen.exile) > 0 {
		exileCards(g, controller, chosen.exile)
		a.paid.exiled = append(a.paid.exiled, chosen.exile...)
	}
	for _, id := range chosen.exileGrave {
		exileFromGraveyard(g, id)
		a.paid.exiled = append(a.paid.exiled, id)
	}
	if len(chosen.discard) > 0 {
		discardCards(g, controller, chosen.discard, pid)
		a.paid.discarded = append(a.paid.discarded, chosen.discard...)
	}
}

func hasDuplicate(ids []CardID) bool {
	for i, id := range ids {
		for _, other := range ids[:i] {
			if id == other {
				return true
			}
		}
	}
	return false
}

func overlaps(a, b []CardID) bool {
	for _, id := range a {
		for _, other := range b {
			if id == other {
				return true
			}
		}
	}
	return false
}
