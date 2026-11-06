// Splice (CR 702.47): as a spell is cast, its caster may reveal cards with
// "Splice onto <type> <cost>" from their hand whose type the spell has; each
// card's text joins the spell's and its splice cost joins the spell's cost.
// The cards stay in the hand.
//
// Ported from forge-game/src/main/java/forge/game/ability/AbilityUtils.java's
// addSpliceEffects and addSpliceEffect (:3185-3256), called from
// PlaySpellAbility.playAbility (:608) after the Charm modes are chosen and
// before X, targets and costs.

package engine

//enginelint:allow id zone card game control valid player ability castspell optionalcost unlesscost effecthelpers

import (
	"fmt"
	"strings"

	"github.com/jczastkiewicz/crucible/internal/keyword"
	"github.com/jczastkiewicz/crucible/internal/valid"
)

// spliceOffer is one card pid may splice onto the spell: its card and the
// splice cost text.
type spliceOffer struct {
	card CardID
	cost string
}

// spliceOffers is addSpliceEffects' candidate list (AbilityUtils.java:
// 3193-3209): each card in pid's hand but the spell itself whose Splice
// keyword's type list ("Arcane", "Instant,Sorcery") the spell matches, in
// hand order. Neither its cost nor anything else is checked here: a splice
// chosen and not paid fails the cast, as Java's does.
func (g *Game) spliceOffers(pid PlayerID, spell *Card) []spliceOffer {
	var out []spliceOffer
	for _, id := range g.Zone(Hand, pid).Cards() {
		c := g.Card(id)
		if id == spell.ID || c.Def == nil || firstSpellAbility(c) == nil {
			continue
		}
		for _, line := range c.KeywordLines() {
			k := keyword.Parse(line)
			if k.Name != "Splice" {
				continue
			}
			onto, cost, ok := strings.Cut(k.Details, ":")
			if ok && Matches(g, spell, valid.Parse(onto), pid, spell.ID) {
				out = append(out, spliceOffer{card: id, cost: cost})
				break
			}
		}
	}
	return out
}

// chooseSplices is addSpliceEffects for a, the spell pid is casting from
// spell: the caster picks any of the offers (chooseCardsForSplice, here
// ChooseEntitiesForEffect), and each picked card's first spell ability joins
// a's tail as a mode (addSpliceEffect: appendSubAbility, the tail Java's
// CharmEffect.chainAbilities also appends to), whose splice cost joins the
// optional costs paid with the spell. Java hosts the spliced ability on the
// spell, not the spliced card (AbilityUtils.java:3246), so its Self and
// CARDNAME name the spell; its SVars are the spliced card's (quirks.md). False
// means the cast stops: a bad choice or a cost this port cannot pay (GO-7).
func (g *Game) chooseSplices(controller PlayerController, pid PlayerID, spell *Card, a *Ability, optional *optionalChoice) bool {
	offers := g.spliceOffers(pid, spell)
	if len(offers) == 0 {
		return true
	}
	options := make([]EntityID, len(offers))
	for i, o := range offers {
		options[i] = CardEntity(o.card)
	}
	chosen := controller.ChooseEntitiesForEffect(g, pid, spell.ID, options, 0, len(options))
	if err := checkChoice(chosen, options, 0, len(options)); err != nil {
		g.recordPendingError(fmt.Errorf("engine: Splice: %w", err))
		return false
	}
	for _, o := range offers {
		picked := false
		for _, e := range chosen {
			picked = picked || e == CardEntity(o.card)
		}
		if !picked {
			continue
		}
		card := g.Card(o.card)
		uc, ok := parseUnlessCost(o.cost)
		if !ok {
			g.recordPendingError(fmt.Errorf("engine: %q: Splice cost %q not resolvable yet", card.Def.Name, o.cost))
			return false
		}
		optional.add(optionalCostOption{text: o.cost, uc: uc})
		line := firstSpellAbility(card)
		api, ok := APIByName(line.Name)
		if !ok {
			g.recordPendingError(fmt.Errorf("engine: %q: spliced ability %q unrecognized", card.Def.Name, line.Name))
			return false
		}
		a.Modes = append(a.Modes, Ability{API: api, Source: spell.ID, Controller: pid, Params: line,
			Amounts: card.Def.Faces[0].Amounts, casting: true, splicedFrom: o.card})
	}
	return true
}

// splicedCards is SpellAbility.getSplicedCards: the cards spliced onto a, in
// the order they were.
func splicedCards(a *Ability) []CardID {
	var out []CardID
	for _, m := range a.Modes {
		if m.splicedFrom != NoCard {
			out = append(out, m.splicedFrom)
		}
	}
	return out
}
