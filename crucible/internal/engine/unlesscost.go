// UnlessCost$ past plain mana: the cost parts a player may pay to prevent an
// effect (AbilityUtils.handleUnlessCost, Ward). PayLife<N>, PayEnergy<N>,
// DamageYou<N>, Draw<N/You>, Discard<N/Card>, Sac<N/Type> and Return<N/Type>
// beside mana tokens are the shapes that need no mid-payment decision beyond
// which cards.

//enginelint:allow id zone card game player ability control event manapay discardeffect sacrificeeffect valid parts returncost combatdamage turn trigger

package engine

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/jczastkiewicz/crucible/internal/cost"
	"github.com/jczastkiewicz/crucible/internal/mana"
	"github.com/jczastkiewicz/crucible/internal/valid"
)

// unlessCost is an UnlessCost$ value this port can pay.
type unlessCost struct {
	parsed cost.Cost
	// mana is the cost's mana tokens; hasMana is false for none.
	mana    mana.Cost
	hasMana bool
	// lifeN, discardN and sacN are the PayLife, Discard<N/Card> and Sac<N/Type>
	// counts, 0 for no such part. sacSpec is the Sac part's valid string, or
	// "CARDNAME" for the source itself.
	lifeN, discardN, sacN int
	sacSpec               string
	// energyN is PayEnergy<N>; mandatory is the Mandatory token, which makes
	// the cost unskippable (CostPart's isMandatory): nobody is asked.
	energyN   int
	mandatory bool
	// damageN is DamageYou<N> (the payer is dealt N noncombat damage by the
	// source), drawN is Draw<N/You>, and returnN/returnSpec are Return<N/Type>
	// (a valid string, or "CARDNAME" for the source itself); 0 for no such part.
	damageN, drawN, returnN int
	returnSpec              string
	// revealN/revealSpec are Reveal<N/Type>: N cards of the type in the payer's
	// hand. Revealing changes no state this port tracks, so paying only needs
	// them to exist (CostReveal.canPay).
	revealN    int
	revealSpec string
	// addCounterN/addCounterType are AddCounter<N/Type>: put N counters of a
	// kind on the source itself (CostPutCounter's CARDNAME shape, Fabricate).
	addCounterN    int
	addCounterType CounterType
}

// parseUnlessCost reads text into an unlessCost, false for a shape past
// mana, PayLife, PayEnergy, DamageYou<N>, Draw<N/You>, Discard<N/Card>,
// AddCounter<N/Type> on the source and one Sac, Return or Reveal part (Tap, an
// X, ...), each its own further mechanic (GO-7).
func parseUnlessCost(text string) (unlessCost, bool) {
	parsed := cost.Parse(text)
	if parsed.Tap || parsed.Untap || parsed.XMin != "" {
		return unlessCost{}, false
	}
	uc := unlessCost{parsed: parsed, mandatory: parsed.Mandatory}
	for _, p := range parsed.Parts {
		n, err := strconv.Atoi(p.Field(0))
		if err != nil || n <= 0 {
			return unlessCost{}, false
		}
		switch {
		case p.Name == "PayLife":
			uc.lifeN += n
		case p.Name == "PayEnergy":
			uc.energyN += n
		case p.Name == "Discard" && p.Field(1) == "Card":
			uc.discardN += n
		case p.Name == "Sac" && uc.sacN == 0 && p.Field(1) != "":
			uc.sacN, uc.sacSpec = n, p.Field(1)
		case p.Name == "Return" && uc.returnN == 0 && p.Field(1) != "":
			uc.returnN, uc.returnSpec = n, p.Field(1)
		case p.Name == "Reveal" && uc.revealN == 0 && revealSpecResolvable(p.Field(1)):
			uc.revealN, uc.revealSpec = n, p.Field(1)
		case p.Name == "AddCounter" && uc.addCounterN == 0 && p.Field(1) != "" && (p.Field(2) == "" || p.Field(2) == "CARDNAME"):
			uc.addCounterN, uc.addCounterType = n, CounterType(strings.ToUpper(p.Field(1)))
		case p.Name == "DamageYou":
			uc.damageN += n
		case p.Name == "Draw" && p.Field(1) == "You":
			uc.drawN += n
		default:
			return unlessCost{}, false
		}
	}
	if len(parsed.Mana) > 0 {
		mc, err := mana.Parse(strings.Join(parsed.Mana, " "))
		if err != nil || mc.CountX() > 0 {
			return unlessCost{}, false
		}
		uc.mana, uc.hasMana = mc, true
	}
	return uc, true
}

// unlessSacCandidates is what pid may sacrifice for the Sac part: the source
// itself for CARDNAME/NICKNAME (when pid controls it and it is not
// phased out, Card.canBeSacrificedBy), else pid's permanents matching the
// valid string.
func (g *Game) unlessSacCandidates(pid PlayerID, source CardID, uc unlessCost) []CardID {
	if uc.sacN == 0 {
		return nil
	}
	if uc.sacSpec == "CARDNAME" || uc.sacSpec == "NICKNAME" {
		c := g.Card(source)
		if c.Zone == Battlefield && !c.IsPhasedOut() && c.Controller() == pid {
			return []CardID{source}
		}
		return nil
	}
	spec := valid.Parse(uc.sacSpec)
	var out []CardID
	for _, id := range g.Zone(Battlefield, pid).Cards() {
		if Matches(g, g.Card(id), spec, g.Card(source).Controller(), source) {
			out = append(out, id)
		}
	}
	return out
}

// unlessReturnCandidates is what pid may return to hand for the Return part:
// the source itself for CARDNAME/NICKNAME (CostReturn's self shape, when pid
// controls it on the battlefield), else pid's permanents matching the valid
// string (returnTypeCandidates, the activation-cost Return<N/Type> twin).
func (g *Game) unlessReturnCandidates(pid PlayerID, source CardID, uc unlessCost) []CardID {
	if uc.returnN == 0 {
		return nil
	}
	if uc.returnSpec == "CARDNAME" || uc.returnSpec == "NICKNAME" {
		c := g.Card(source)
		if c.Zone == Battlefield && c.Controller() == pid {
			return []CardID{source}
		}
		return nil
	}
	return returnTypeCandidates(g, pid, source, uc.returnSpec)
}

// revealSpecResolvable is false for CostReveal's own special types (the whole
// "Hand", "SameColor", the source itself), each a different payment.
func revealSpecResolvable(spec string) bool {
	switch spec {
	case "", "Hand", "SameColor", "CARDNAME", "NICKNAME":
		return false
	}
	return true
}

// unlessRevealable reports whether pid's hand holds the cards the Reveal part
// names: revealN cards matching revealSpec, an OR list when it carries ";"
// (the Cost syntax's own comma, as returnTypeCandidates reads it).
func (g *Game) unlessRevealable(pid PlayerID, source CardID, uc unlessCost) bool {
	if uc.revealN == 0 {
		return true
	}
	spec := valid.Parse(strings.ReplaceAll(uc.revealSpec, ";", ","))
	n := 0
	for _, id := range g.Zone(Hand, pid).Cards() {
		if Matches(g, g.Card(id), spec, pid, source) {
			n++
		}
	}
	return n >= uc.revealN
}

// payUnlessCost pays uc for pid, reporting whether it was paid. Every part is
// checked payable first -- life at least PayLife (CR 119.4), enough cards to
// discard, enough permanents to sacrifice -- and the mana is paid next, which
// fails atomically (PayManaCost), so a cost that cannot be met leaves
// everything as it was. Then life is paid (the LifeChanged event
// loseLifeEffect emits), the controller chooses the discards and the
// sacrifices.
func (g *Game) payUnlessCost(controller PlayerController, a *Ability, pid PlayerID, uc unlessCost) bool {
	if !g.unlessPayable(pid, a.Source, uc) {
		return false
	}
	if uc.hasMana && !g.PayManaCost(pid, uc.mana, controller) {
		return false
	}
	g.payUnlessParts(controller, a, pid, uc)
	return true
}

// unlessPayable reports whether every non-mana part of uc can be paid by pid:
// life, energy, cards to discard, permanents to sacrifice or return, a card to
// reveal and a draw (CostPart.canPay for each).
func (g *Game) unlessPayable(pid PlayerID, source CardID, uc unlessCost) bool {
	hand := handWithout(g.Zone(Hand, pid).Cards(), source)
	candidates := g.unlessSacCandidates(pid, source, uc)
	returnable := g.unlessReturnCandidates(pid, source, uc)
	return uc.lifeN <= g.Player(pid).Life && (uc.lifeN == 0 || !g.cantPayLife(pid, false, causeNone)) && uc.energyN <= g.Player(pid).Counters.Count(Energy) &&
		uc.discardN <= len(hand) && uc.sacN <= len(candidates) && uc.returnN <= len(returnable) &&
		(uc.drawN == 0 || !g.cantDrawAmount(pid, uc.drawN)) && g.unlessRevealable(pid, source, uc) &&
		g.unlessCounterable(source, uc)
}

// unlessCounterable is CostPutCounter.canPay for the source itself: it is on the
// battlefield and may receive the counters.
func (g *Game) unlessCounterable(source CardID, uc unlessCost) bool {
	if uc.addCounterN == 0 {
		return true
	}
	return g.Card(source).Zone == Battlefield && !g.cantPutCounter(CardEntity(source), uc.addCounterType)
}

// payUnlessParts pays uc's non-mana parts, after unlessPayable said they can
// be: life (the LifeChanged event loseLifeEffect emits), energy, the controller's
// discards and sacrifices, returns, damage to the payer and draws.
func (g *Game) payUnlessParts(controller PlayerController, a *Ability, pid PlayerID, uc unlessCost) {
	hand := handWithout(g.Zone(Hand, pid).Cards(), a.Source)
	candidates := g.unlessSacCandidates(pid, a.Source, uc)
	returnable := g.unlessReturnCandidates(pid, a.Source, uc)
	if uc.lifeN > 0 {
		g.Player(pid).Life -= uc.lifeN
		g.sink.Emit(Event{Kind: LifeChanged, Source: a.Source, Target: PlayerEntity(pid), Amount: -int32(uc.lifeN)})
	}
	if uc.energyN > 0 {
		g.Player(pid).Counters.Add(Energy, -uc.energyN)
		emitCounterChanged(g.sink, a.Source, PlayerEntity(pid), Energy, -uc.energyN)
	}
	if uc.discardN > 0 {
		discardCards(g, controller, controller.ChooseCardsToDiscard(g, pid, hand, uc.discardN), pid)
	}
	if uc.sacN > 0 {
		sacrificeCardsFor(g, controller, a, controller.ChoosePermanentsToSacrifice(g, pid, candidates, uc.sacN), false)
	}
	if uc.returnN > 0 {
		returnCards(g, controller, controller.ChoosePermanentsToReturn(g, pid, returnable, uc.returnN))
	}
	if uc.damageN > 0 {
		var table damageTable
		g.dealPlayerDamage(controller, a.Source, pid, uc.damageN, false, &table)
		g.checkDamageTableTriggers(controller, table, false)
	}
	if uc.drawN > 0 {
		g.DrawCards(pid, uc.drawN, controller)
	}
	if uc.addCounterN > 0 {
		if n := g.countersReplaced(controller, pid, CardEntity(a.Source), uc.addCounterType, uc.addCounterN); n > 0 {
			g.addCardCounters(controller, a.Source, a.Source, uc.addCounterType, n)
		}
	}
}

// handWithout is hand less source: a spell being cast cannot be discarded to pay
// for itself (CostDiscard leaves the host out while it is a spell).
func handWithout(hand []CardID, source CardID) []CardID {
	out := make([]CardID, 0, len(hand))
	for _, id := range hand {
		if id != source {
			out = append(out, id)
		}
	}
	return out
}

// times is uc paid n times over, Cost.mergeTo(cost, n, sa) for Cumulative
// upkeep: every amount multiplied, the mana repeated. False when the mana does
// not parse again.
func (uc unlessCost) times(n int) (unlessCost, bool) {
	uc.lifeN *= n
	uc.discardN *= n
	uc.sacN *= n
	uc.energyN *= n
	uc.damageN *= n
	uc.drawN *= n
	uc.returnN *= n
	uc.revealN *= n
	uc.addCounterN *= n
	if uc.hasMana {
		var tokens []string
		for range n {
			tokens = append(tokens, uc.parsed.Mana...)
		}
		mc, err := mana.Parse(strings.Join(tokens, " "))
		if err != nil {
			return unlessCost{}, false
		}
		uc.mana = mc
	}
	// What the payer is asked to confirm is the whole cost: the text n times
	// over, as Cost.mergeTo adds it.
	var repeated []string
	for range n {
		repeated = append(repeated, uc.parsed.Text)
	}
	uc.parsed = cost.Parse(strings.Join(repeated, " "))
	return uc, true
}

// upkeepCostPaid is the Echo$ and CumulativeUpkeep$ branches of
// SacrificeEffect.resolve: pay the cost or lose the permanent. proceed is true
// when the sacrifice goes ahead -- the cost was not paid and the permanent is
// still its activator's -- and false for an ability with neither param
// (ordinary sacrifice, proceed true), a paid cost, or a permanent that
// changed controller since the trigger.
func (g *Game) upkeepCostPaid(controller PlayerController, a *Ability, source *Card) (proceed bool, err error) {
	echo, isEcho := a.Params.Param("Echo")
	cumulative, isCumulative := a.Params.Param("CumulativeUpkeep")
	if !isEcho && !isCumulative {
		return true, nil
	}
	text, n := echo, 1
	if isCumulative {
		text = cumulative
		// One more AGE counter, through any AddCounter replacement, before
		// the cost is built from how many there are.
		if added := g.countersReplaced(controller, a.Controller, CardEntity(source.ID), Age, 1); added > 0 {
			g.addCardCounters(controller, source.ID, source.ID, Age, added)
		}
		n = source.Counters.Count(Age)
		if n == 0 {
			// SacrificeEffect.java:57: no age counter, no cost to pay.
			return false, nil
		}
	}
	uc, ok := parseUnlessCost(text)
	if ok && n > 1 {
		uc, ok = uc.times(n)
	}
	if !ok {
		return false, fmt.Errorf("engine: Sacrifice: upkeep cost %q not resolvable yet", text)
	}
	paid := (uc.mandatory || controller.ConfirmPayCost(g, a.Controller, uc.parsed, a.Source)) &&
		g.payUnlessCost(controller, a, a.Controller, uc)
	return !paid && source.Controller() == a.Controller, nil
}
