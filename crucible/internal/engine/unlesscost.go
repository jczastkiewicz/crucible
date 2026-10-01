// UnlessCost$ past plain mana: the cost parts a player may pay to prevent an
// effect (AbilityUtils.handleUnlessCost, Ward). PayLife<N>, PayEnergy<N>,
// DamageYou<N>, Draw<N/You>, Discard<N/Card>, Sac<N/Type> and Return<N/Type>
// beside mana tokens are the shapes that need no mid-payment decision beyond
// which cards.

//enginelint:allow id zone card game player ability control event manapay discardeffect sacrificeeffect valid parts returncost combatdamage turn trigger

package engine

import (
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
}

// parseUnlessCost reads text into an unlessCost, false for a shape past
// mana, PayLife, PayEnergy, DamageYou<N>, Draw<N/You>, Discard<N/Card> and one
// Sac, Return or Reveal part (Tap, an X, ...), each its own further mechanic
// (GO-7).
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
	hand := g.Zone(Hand, pid).Cards()
	candidates := g.unlessSacCandidates(pid, a.Source, uc)
	returnable := g.unlessReturnCandidates(pid, a.Source, uc)
	if uc.lifeN > g.Player(pid).Life || uc.energyN > g.Player(pid).Counters.Count(Energy) ||
		uc.discardN > len(hand) || uc.sacN > len(candidates) || uc.returnN > len(returnable) ||
		!g.unlessRevealable(pid, a.Source, uc) {
		return false
	}
	if uc.hasMana && !g.PayManaCost(pid, uc.mana, controller) {
		return false
	}
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
		sacrificeCards(g, controller, a, controller.ChoosePermanentsToSacrifice(g, pid, candidates, uc.sacN))
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
	return true
}
