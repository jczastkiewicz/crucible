// Suspend (CR 702.62): "Suspend N--<cost>" lets the owner of the card in their
// hand pay <cost> and exile it with N time counters, a special action (CR
// 116.2f) taken any time they could cast the card. keyword.ExpandSuspend's two
// triggers remove a time counter at each of their upkeeps and, as the last one
// is removed, let them cast it without paying its mana cost, a creature with
// haste.
//
// Ported from CardFactoryUtil.java:3548-3607, an AbilityStatic in the hand
// (resolved at once, never on the stack, MagicStack.java:289-292) whose
// canPlay is the card's first spell's timing (:3555-3569) and whose resolve
// exiles the card and adds the counters as a special action, through the
// AddCounter replacements (:3572-3589). A count or cost naming X (5 corpus
// cards, Java reads the card's own X SVar) is not offered (GO-7).

package engine

//enginelint:allow id card game ability control zone zonemove manapay manarestrict unlesscost counteradded priority splitsecond staticability parts altcost

import (
	"strconv"
	"strings"

	"github.com/jczastkiewicz/crucible/internal/cardtype"
	"github.com/jczastkiewicz/crucible/internal/keyword"
)

// suspendWay is c's "Suspend:<N>:<cost>": the time counters and the cost
// text. ok is false for a bare Suspend (a granted one, which has no special
// action, :3548) and for an X in either field: Java reads X from the
// cast SA, which the special action never sets (forge-java-defects.md).
func suspendWay(c *Card) (n int, cost string, ok bool) {
	for _, line := range c.KeywordLines() {
		k := keyword.Parse(line)
		if k.Name != "Suspend" {
			continue
		}
		args := k.Args()
		if len(args) < 2 || args[1] == "" || strings.Contains(args[1], "X") {
			continue
		}
		n, err := strconv.Atoi(args[0])
		if err != nil {
			continue
		}
		return n, args[1], true
	}
	return 0, "", false
}

// Suspend is the special action: pid pays card's suspend cost and exiles it
// from their hand with its N time counters. False when card is not in pid's
// hand, has no suspend cost this port pays, could not be cast now (its
// timing, split second, a CantBeCast line), or the cost is not paid.
func (g *Game) Suspend(pid PlayerID, card CardID, controller PlayerController) bool {
	c := g.Card(card)
	if c.Def == nil || c.Zone != Hand || c.Owner != pid || g.splitSecondOnStack() || g.cantBeCast(pid, card) {
		return false
	}
	if !c.Type().Has(cardtype.Instant) && !c.HasKeyword("Flash") && !g.castsWithFlash(pid, card) && !g.canActSorcerySpeed(pid) {
		return false
	}
	n, text, ok := suspendWay(c)
	if !ok {
		return false
	}
	uc, ok := parseUnlessCost(text)
	if !ok {
		return false
	}
	act := &Ability{Source: card, Controller: pid}
	if hasNonManaParts(uc) {
		if !g.unlessPayable(pid, card, uc) || !g.payUnlessParts(controller, act, pid, uc) {
			return false
		}
	}
	if uc.hasMana {
		restore := g.payingFor(&payContext{item: act, cost: uc.mana})
		_, paid := g.payManaCostX(pid, uc.mana, controller)
		restore()
		if !paid {
			return false
		}
	}
	g.moveByEffect(controller, card, Exile, 0, NoPlayer, false)
	g.checkChangesZoneAllTriggers(controller, []CardID{card}, Hand, Exile)
	if g.Card(card).Zone != Exile {
		return true
	}
	if n = g.countersReplaced(controller, pid, CardEntity(card), Time, n); n > 0 {
		g.addCardCounters(controller, pid, card, card, Time, n)
	}
	return true
}
