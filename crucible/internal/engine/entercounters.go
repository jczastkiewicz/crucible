// "Enters with counters": the Event$ Moved replacements that put counters on a
// permanent as it enters the battlefield (CR 614.1c, CR 122.6), written as a
// ReplaceWith$ ability `DB$ PutCounter | ETB$ True`. CardFactoryUtil's
// makeEtbCounter builds the same shape for `K:etbCounter:<type>:<amount>`
// (keyword.Expand, ADR-0038).
//
// Java's PutCounterEffect with ETB$ puts the counters into the replaced
// event's counter table, which the entering permanent receives as it enters;
// here they are added right after the move, before any "enters" trigger looks
// at the permanent (enterBattlefieldReplacements), through countersReplaced so
// an AddCounter replacement (Hardened Scales) still changes them.

//enginelint:allow id zone card game valid ability replacement replaceeffect amount putcountereffect control event parts

package engine

import (
	"fmt"
	"strings"

	"github.com/jczastkiewicz/crucible/internal/carddb/compile"
	"github.com/jczastkiewicz/crucible/internal/expr"
	"github.com/jczastkiewicz/crucible/internal/valid"
)

// applyEnterCounters applies every counter-adding Moved replacement that
// matches moved's entry onto the battlefield from origin: moved's own
// ("CARDNAME enters with N counters", Defined$ Self) and another permanent's
// ("each creature you control enters with an additional counter", Defined$
// ReplacedCard). Unlike the tap shape, each of them applies, since counters
// from separate effects add up (CR 616.1's choice only orders them).
//
// A line this port cannot evaluate records a pending error rather than
// entering without the counters (ADR-0020 decision 4, GO-7): a PutCounter with
// a param past Defined$/CounterType$/CounterNum$/ETB$, a Defined$ other than
// Self or ReplacedCard, an amount that does not resolve.
func (g *Game) applyEnterCounters(controller PlayerController, moved CardID, origin ZoneType) {
	movedCard := g.Card(moved)
	// Java puts every ETB$ counter into one counter table and runs the
	// AddCounter replacements on it once, so the amounts of one kind are summed
	// first (Hardened Scales adds one for the lot, not one per line).
	type pile struct {
		ct     CounterType
		n      int
		source CardID
		placer PlayerID
	}
	var piles []pile
	g.eachReplacement("Moved", func(h *Card, amounts map[string]expr.Amount, r *compile.Ability) bool {
		sub := replaceWithSub(r)
		if sub == nil || !strings.EqualFold(sub.Name, "PutCounter") {
			return false
		}
		if _, ok := sub.Param("ETB"); !ok {
			return false
		}
		if !replacementZoneMatches(r, "Destination", Battlefield) || !replacementZoneMatches(r, "Origin", origin) {
			return false
		}
		validCard, ok := r.Param("ValidCard")
		if !ok {
			// ReplaceMoved matches any card with no ValidCard$; no corpus line
			// writes that for counters, so it is not guessed at.
			g.recordPendingError(fmt.Errorf("engine: %q: ETB$ PutCounter replacement without ValidCard$ not resolvable yet", h.Def.Name))
			return false
		}
		if !Matches(g, movedCard, valid.Parse(validCard), h.Controller(), h.ID) {
			return false
		}
		if !replacementRequirementsCheck(g, h, amounts, r) {
			return false
		}
		if !onlyParams(r, "validcard", "destination", "origin", "replacementresult", "layer",
			// the flags triggerCommonRequirementsMet evaluates
			"bloodthirst", "metalcraft", "delirium", "threshold", "hellbent", "fatefulhour", "lifetotal", "lifeamount",
			"ispresent2", "presentcompare2", "presentzone2", "presentplayer2", "presentdefined2") {
			// Optional$ and the like: counters it may decline are not put
			// unconditionally.
			g.recordPendingError(fmt.Errorf("engine: %q: ETB$ PutCounter replacement: a param past ValidCard$/Destination$/Origin$ not resolvable yet", h.Def.Name))
			return false
		}
		ct, n, err := g.enterCounterAmount(moved, h, amounts, sub)
		if err != nil {
			g.recordPendingError(err)
			return false
		}
		if n <= 0 {
			return false
		}
		for i := range piles {
			if piles[i].ct == ct {
				piles[i].n += n
				return false
			}
		}
		piles = append(piles, pile{ct: ct, n: n, source: h.ID, placer: h.Controller()})
		return false
	})
	for _, p := range piles {
		if n := g.countersReplaced(controller, p.placer, CardEntity(moved), p.ct, p.n); n > 0 {
			g.addCardCounters(controller, p.source, moved, p.ct, n)
		}
	}
}

// enterCounterAmount reads one ETB$ PutCounter's counter kind and amount for
// moved, 0 amount for a line that puts nothing on it (a watcher's own "Self").
func (g *Game) enterCounterAmount(moved CardID, host *Card, amounts map[string]expr.Amount, sub *compile.Ability) (CounterType, int, error) {
	name := host.Def.Name
	if !onlyKeys(sub, "DB", "Defined", "CounterType", "CounterNum", "ETB", "SpellDescription") {
		return "", 0, fmt.Errorf("engine: %q: ETB$ PutCounter: a param past Defined$/CounterType$/CounterNum$ not resolvable yet", name)
	}
	defined, _ := sub.Param("Defined")
	switch {
	case defined == "Self" && host.ID == moved, defined == "ReplacedCard":
	case defined == "Self":
		// A watcher's own "Self" is the watcher, not what entered.
		return "", 0, nil
	default:
		return "", 0, fmt.Errorf("engine: %q: ETB$ PutCounter: Defined$ %q not resolvable yet", name, defined)
	}
	counterType, err := putCounterType(sub)
	if err != nil {
		return "", 0, fmt.Errorf("engine: %q: %w", name, err)
	}
	counterNum, ok := sub.Param("CounterNum")
	if !ok {
		counterNum = "1"
	}
	amount, ok := resolveNamedAmount(g, amounts, host, counterNum)
	if !ok {
		return "", 0, fmt.Errorf("engine: %q: ETB$ PutCounter: CounterNum$ %q is not resolvable", name, counterNum)
	}
	return counterType, amount, nil
}
