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

//enginelint:allow id zone card game valid ability replacement replaceeffect amount putcountereffect control event

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
		if !ok || !Matches(g, movedCard, valid.Parse(validCard), h.Controller(), h.ID) {
			return false
		}
		if !replacementRequirementsCheck(g, h, amounts, r) {
			return false
		}
		if !onlyParams(r, "validcard", "destination", "origin", "replacementresult", "layer") {
			// Optional$ and the like: counters it may decline are not put
			// unconditionally.
			g.recordPendingError(fmt.Errorf("engine: %q: ETB$ PutCounter replacement: a param past ValidCard$/Destination$/Origin$ not resolvable yet", h.Def.Name))
			return false
		}
		if err := g.putEnterCounters(controller, moved, h, amounts, sub); err != nil {
			g.recordPendingError(err)
		}
		return false
	})
}

// putEnterCounters puts one ETB$ PutCounter's counters on moved.
func (g *Game) putEnterCounters(controller PlayerController, moved CardID, host *Card, amounts map[string]expr.Amount, sub *compile.Ability) error {
	name := host.Def.Name
	if !onlyKeys(sub, "DB", "Defined", "CounterType", "CounterNum", "ETB", "SpellDescription") {
		return fmt.Errorf("engine: %q: ETB$ PutCounter: a param past Defined$/CounterType$/CounterNum$ not resolvable yet", name)
	}
	defined, _ := sub.Param("Defined")
	switch {
	case defined == "Self" && host.ID == moved, defined == "ReplacedCard":
	case defined == "Self":
		// A watcher's own "Self" is the watcher, not what entered.
		return nil
	default:
		return fmt.Errorf("engine: %q: ETB$ PutCounter: Defined$ %q not resolvable yet", name, defined)
	}
	counterType, err := putCounterType(sub)
	if err != nil {
		return fmt.Errorf("engine: %q: %w", name, err)
	}
	counterNum, ok := sub.Param("CounterNum")
	if !ok {
		counterNum = "1"
	}
	amount, ok := resolveNamedAmount(g, amounts, host, counterNum)
	if !ok {
		return fmt.Errorf("engine: %q: ETB$ PutCounter: CounterNum$ %q is not resolvable", name, counterNum)
	}
	if amount <= 0 {
		return nil
	}
	n := g.countersReplaced(controller, host.Controller(), CardEntity(moved), counterType, amount)
	if n <= 0 {
		return nil
	}
	g.Card(moved).Counters.Add(counterType, n)
	emitCounterChanged(g.sink, host.ID, CardEntity(moved), counterType, n)
	return nil
}
