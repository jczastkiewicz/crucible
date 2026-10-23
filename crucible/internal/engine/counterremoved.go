// Mode$ CounterRemoved and Mode$ CounterRemovedOnce: the triggers a counter
// taken off a card sets off (Card.subtractCounter, Card.java:1886-1948).
// Removal sites call emitCounterChanged with no controller to push to, so the
// matches are found there, as the counters come off, and wait in
// Game.pendingCounterRemoved until the next state-based action check puts them
// on the stack, before any player gets priority (CR 603.3).

//enginelint:allow id zone card game valid ability event amount parts trigger statictrigger control

package engine

import (
	"strconv"
	"strings"

	"github.com/jczastkiewicz/crucible/internal/carddb/compile"
	"github.com/jczastkiewicz/crucible/internal/valid"
)

// counterRemovedParams are the params this port reads on each mode. A
// CounterRemoved line naming ValidPlayer$ is skipped: the remover is not
// recorded by the removal sites, and Java fails the test for a removal with
// no remover.
var counterRemovedParams = map[string][]string{
	"CounterRemoved":     {"ValidCard", "CounterType", "NewCounterAmount"},
	"CounterRemovedOnce": {"ValidCard", "CounterType", "Remaining"},
}

// queueCounterRemoved records the CounterRemoved and CounterRemovedOnce
// triggers n counters of ct leaving card id fire (Card.java:1935-1945): one
// CounterRemoved per counter, NewCounterAmount counting down from the old total
// less one to the new one, then one CounterRemovedOnce for the whole removal.
// The card already holds its new total.
func (g *Game) queueCounterRemoved(id CardID, ct CounterType, n int) {
	if n <= 0 {
		return
	}
	now := g.Card(id).Counters.Count(ct)
	for left := now + n - 1; left >= now; left-- {
		g.pendingCounterRemoved = g.counterRemovedMatches(g.pendingCounterRemoved, "CounterRemoved", id, ct, left, n)
	}
	g.pendingCounterRemoved = g.counterRemovedMatches(g.pendingCounterRemoved, "CounterRemovedOnce", id, ct, now, n)
}

// flushCounterRemoved puts the queued counter-removal triggers on the stack.
func (g *Game) flushCounterRemoved(controller PlayerController) {
	if len(g.pendingCounterRemoved) == 0 {
		return
	}
	matches := g.pendingCounterRemoved
	g.pendingCounterRemoved = nil
	g.pushTriggeredAbilities(controller, matches)
}

// counterRemovedMatches appends the triggers of mode that a removal fires,
// newAmount being the card's total after this counter (CounterRemoved) or the
// whole removal (CounterRemovedOnce), removed the number taken off.
func (g *Game) counterRemovedMatches(matches []Ability, mode string, id CardID, ct CounterType, newAmount, removed int) []Ability {
	return append(matches, g.scanTriggers([]string{mode}, func(h *Card, _ triggerFace, t *compile.Ability) (triggeredObjects, bool) {
		if !counterRemovedReadable(t, mode) {
			return triggeredObjects{}, false
		}
		if vc, ok := t.Param("ValidCard"); ok && !Matches(g, g.Card(id), valid.Parse(vc), h.Controller(), h.ID) {
			return triggeredObjects{}, false
		}
		if want, ok := t.Param("CounterType"); ok && !strings.EqualFold(want, string(ct)) {
			return triggeredObjects{}, false
		}
		key := "NewCounterAmount"
		if mode == "CounterRemovedOnce" {
			key = "Remaining"
		}
		if v, ok := t.Param(key); ok {
			if n, err := strconv.Atoi(v); err != nil || n != newAmount {
				return triggeredObjects{}, false
			}
		}
		objects := triggeredObjects{card: id}
		if mode == "CounterRemovedOnce" {
			objects.counts = triggerCounts{amount: removed, set: countAmount}
		}
		return objects, true
	})...)
}

// counterRemovedReadable reports whether every param of t is one this port
// evaluates for mode or one the general trigger gates read.
func counterRemovedReadable(t *compile.Ability, mode string) bool {
	for _, p := range t.Params {
		switch strings.ToLower(p.Key) {
		case "mode", "execute", "triggerdescription", "secondary", "triggerzones",
			"optionaldecider", "ispresent", "presentcompare", "presentzone", "presentplayer",
			"checksvar", "svarcompare", "playerturn", "phase", "opponentturn", "notplayerturn":
			continue
		}
		known := false
		for _, k := range counterRemovedParams[mode] {
			known = known || strings.EqualFold(p.Key, k)
		}
		if !known {
			return false
		}
	}
	return true
}
