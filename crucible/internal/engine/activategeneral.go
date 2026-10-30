// Activating an ability whose Cost$ ActivateAbility's own shape reader
// (cost.ActivationShape) does not cover: a counter removed from another
// permanent, two sacrifices, an untap symbol, Waterbend, a revealed card...
// The cost is read by the unless-cost reader (parseUnlessCost and its X
// handling, triggeredCostShape) that spells, triggered abilities and optional
// costs already pay through, with the {T} and {Q} symbols paid here. Ported
// from CostPayment.payCost over the ability's Cost (CostPayment.java), after
// PlaySpellAbility.playAbility's announce and target steps.

package engine

//enginelint:allow id card game player ability control parts zone castspell manarestrict taptype unlesscost

import (
	"fmt"
	"strings"

	"github.com/jczastkiewicz/crucible/internal/carddb/compile"
	"github.com/jczastkiewicz/crucible/internal/cost"
)

// activateGeneral is ActivateAbility for a cost the shape reader refused, from
// the battlefield or the hand (Forecast). A cost the unless-cost reader cannot read
// either refuses the activation with a pending error (GO-7); one it reads but
// the player cannot pay refuses it silently, as any unaffordable cost does.
func (g *Game) activateGeneral(controller PlayerController, pid PlayerID, card CardID, index int, ability *compile.Ability,
	costText string, fromGraveyard, isLoyaltyAbility bool) (done bool) {
	c := g.Card(card)
	xMin, costText := splitXMin(costText)
	parsed := cost.Parse(costText)
	// A graveyard ability's parts would move or exile the card the payment
	// reads; none of the shapes this path pays is built for it.
	if fromGraveyard || parsed.Mandatory || (parsed.Tap || parsed.Untap) && c.Zone != Battlefield {
		g.recordPendingError(fmt.Errorf("engine: %s: activated Cost$ %q not resolvable yet", c.Def.Name, costText))
		return false
	}
	// {T} and {Q}: the host is tapped or untapped as part of the cost, and a
	// summoning-sick creature pays neither (CR 302.6).
	if (parsed.Tap || parsed.Untap) && c.isSick() && !g.asIfHaste("ActivateAbilityAsIfHaste", c, NoEntity) {
		return false
	}
	if parsed.Tap && c.Tapped || parsed.Untap && !c.Tapped {
		return false
	}
	apiType, ok := APIByName(ability.Name)
	if !ok {
		return false
	}
	activated := Ability{
		API: apiType, Source: card, Controller: pid,
		Params: ability, Amounts: c.abilityAmounts(ability), costPaid: true, activated: true,
		activationIndex: index + 1,
	}
	rest := withoutTapSymbols(costText)
	uc := unlessCost{}
	if xMin > 0 && costNamesX(rest) && g.nonManaXMax(pid, card, rest) < xMin {
		// Cost.getMaxForNonManaX (:1126-1128): the parts cannot pay the
		// floor, so the ability cannot be activated at all.
		return false
	}
	if rest != "" {
		var announce bool
		var err error
		if uc, announce, err = g.triggeredCostShape(&activated, rest, controller); err != nil {
			g.recordPendingError(fmt.Errorf("engine: %s: activated Cost$ %q not resolvable yet", c.Def.Name, costText))
			return false
		}
		if announce {
			if uc, ok = g.announceTriggeredX(controller, &activated, rest, uc); !ok {
				return false
			}
			if activated.xManaCostPaid < xMin {
				// CostPartMana.getXMin: X can't be below the floor.
				g.recordPendingError(fmt.Errorf("engine: X %d is below the cost's XMin%d", activated.xManaCostPaid, xMin))
				return false
			}
		}
	}
	if !g.unlessPayable(pid, card, uc) {
		return false
	}
	if !g.chooseAbilityTargets(controller, &activated) {
		return false
	}
	if uc.hasMana {
		savedResolving := g.resolving
		g.resolving = &activated
		uc.mana, ok = g.costStatics(pid, card, ability, uc.mana, true, false)
		g.resolving = savedResolving
		if !ok {
			return false
		}
	}
	before := *c
	outerHold, outerExits := g.holdingCastExits, g.castExits
	g.holdingCastExits, g.castExits = true, nil
	if parsed.Tap {
		c.Tapped = true
	}
	if parsed.Untap {
		c.Tapped = false
	}
	if !g.payActivatedUnlessCost(controller, &activated, pid, uc) {
		// Nothing past the mana was paid (payUnlessCostInto checks every part
		// before it pays any): the symbol goes back.
		g.Card(card).Tapped = before.Tapped
		g.holdingCastExits, g.castExits = outerHold, outerExits
		return false
	}
	activated.paid.recorded = true
	if parsed.Tap {
		g.checkTapsTriggers(controller, card, pid, false)
		g.checkTapAllTriggers(controller, []CardID{card})
	}
	if parsed.Untap {
		g.checkUntapsTriggers(controller, card)
	}
	return g.finishActivation(controller, pid, card, index, ability, &activated, &before, isLoyaltyAbility, false, outerHold, outerExits)
}

// payActivatedUnlessCost is payUnlessCostInto for an activation: its mana is
// paid for the ability being activated (not one resolving, so restricted mana
// may pay it), then its other parts, each recorded in the ability's paid
// lists.
func (g *Game) payActivatedUnlessCost(controller PlayerController, a *Ability, pid PlayerID, uc unlessCost) bool {
	mc, taps := uc.mana, []CardID(nil)
	if uc.waterbendN > 0 {
		mc, taps = g.waterbendReduce(controller, pid, a.Source, uc)
	}
	if uc.hasMana {
		restore := g.payingFor(&payContext{item: a, cost: mc})
		x, paid := g.payManaCostX(pid, mc, controller)
		restore()
		if !paid {
			return false
		}
		if x.announced {
			// An X announced before the cost was read (announceTriggeredX)
			// is already on a; a mana X announced here records it.
			x.setOn(a)
		}
		a.paid.mana, a.paid.manaRecorded = x.sources, true
		a.paid.colors, a.paid.colorless = x.paid, x.colorless
	}
	tapChosenPermanents(g, controller, taps)
	return g.payUnlessPartsInto(controller, a, pid, uc, &a.paid)
}

// withoutTapSymbols is a Cost$ text with its T and Q tokens taken out.
func withoutTapSymbols(text string) string {
	var kept []string
	for _, f := range strings.Fields(text) {
		if f != "T" && f != "Q" {
			kept = append(kept, f)
		}
	}
	return strings.Join(kept, " ")
}
