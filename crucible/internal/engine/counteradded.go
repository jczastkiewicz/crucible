// Mode$ CounterAdded and Mode$ CounterAddedOnce: the triggers a counter put on
// a card sets off (Card.addCounter, Card.java:1780-1822). Every effect that puts
// counters on a card goes through addCardCounters, so a Saga's chapters, "whenever
// one or more +1/+1 counters are put on" and the rest see every placement.

//enginelint:allow id zone card game valid ability control event amount parts trigger statictrigger triggerpack triggerscan staticability

package engine

import (
	"strconv"
	"strings"

	"github.com/jczastkiewicz/crucible/internal/carddb/compile"
	"github.com/jczastkiewicz/crucible/internal/valid"
)

// addCardCounters puts n counters of ct on card id, emits the event, then
// fires the triggers: Mode$ CounterAdded once per counter, with the new total
// as CounterAmount, and Mode$ CounterAddedOnce once for the whole placement.
// placer is the player putting them, Card.addCounterInternal's own
// `Player source` (Card.java:1761): what the triggers' ValidSource$ and the
// turn's counter record read (AbilityKey.Source, Card.java:1805;
// Game.addCounterAddedThisTurn, Card.java:1792). A caller that runs the
// AddCounter replacements first (countersReplaced) gives them the same
// player, so the replacement and the trigger agree on who placed. source is the card the counters came from, for
// the event (NoCard for a turn-based action). n <= 0 does nothing.
func (g *Game) addCardCounters(controller PlayerController, placer PlayerID, source CardID, id CardID, ct CounterType, n int) {
	if n <= 0 {
		return
	}
	c := g.Card(id)
	old := c.Counters.Count(ct)
	// Card.addCounterInternal (Card.java:1760): a Mode$ MaxCounter static
	// caps the new total, CR 704.5r's Dream counters.
	if limit, capped := g.maxCounter(id, ct); capped {
		n = min(n, limit-old)
		if n <= 0 {
			return
		}
	}
	first := !g.countersAddedToThisTurn(id)
	c.Counters.Add(ct, n)
	g.recordCountersAdded(placer, id, ct, n)
	emitCounterChanged(g, source, CardEntity(id), ct, n)
	var matches []Ability
	for i := 1; i <= n; i++ {
		matches = g.counterAddedMatches(matches, "CounterAdded", placer, CardEntity(id), ct, old+i)
	}
	matches = g.counterAddedMatches(matches, "CounterAddedOnce", placer, CardEntity(id), ct, n)
	g.pushTriggeredAbilities(controller, matches)
	g.checkCounterBatchTriggers(controller, placer, id, ct, n, first)
}

// putPlayerCounters is a player getting n counters of ct from an effect, the
// way GameEntityCounterTable.replaceCounterEffect places them: the
// AddCounter replacements first (countersReplaced: Melira's one poison
// counter, Winding Constrictor's one more, Vorinclex doubling), then
// addPlayerCounters. It returns how many were put.
func (g *Game) putPlayerCounters(controller PlayerController, placer PlayerID, source CardID, pid PlayerID, ct CounterType, n int) int {
	n = g.countersReplaced(controller, placer, PlayerEntity(pid), ct, n)
	g.addPlayerCounters(controller, placer, source, pid, ct, n)
	return max(n, 0)
}

// addPlayerCounters is Player.addCounterInternal (Player.java:858-890): n
// counters of ct on pid, the event, the Radiation effect card a first rad
// counter brings (createRadiationEffect), then the triggers -- Mode$
// CounterAdded once per counter with the new total, Mode$ CounterAddedOnce
// once (Aether Revolt, Fabrication Module: "whenever you get one or more
// {E}"), and Mode$ CounterPlayerAddedAll for the placer
// (GameEntityCounterTable.triggerCountersPutAll; All Will Be One). Java logs
// no player placement in the turn's counter record (Game.addCounterAddedThisTurn
// is Card-only), so neither does this. n <= 0 does nothing (CR 107.1b).
func (g *Game) addPlayerCounters(controller PlayerController, placer PlayerID, source CardID, pid PlayerID, ct CounterType, n int) {
	if n <= 0 {
		return
	}
	p := g.Player(pid)
	old := p.Counters.Count(ct)
	p.Counters.Add(ct, n)
	emitCounterChanged(g, source, PlayerEntity(pid), ct, n)
	if ct == Radiation {
		g.syncRadiationEffect(pid)
	}
	var matches []Ability
	for i := 1; i <= n; i++ {
		matches = g.counterAddedMatches(matches, "CounterAdded", placer, PlayerEntity(pid), ct, old+i)
	}
	matches = g.counterAddedMatches(matches, "CounterAddedOnce", placer, PlayerEntity(pid), ct, n)
	matches = append(matches, g.counterPlayerAddedAllMatches(placer, PlayerEntity(pid), n)...)
	g.pushTriggeredAbilities(controller, matches)
}

// payCounterCost is CostPutCounter's own placement on the source
// (CostPutCounter.java:182, :201-207): the payer puts n counters of ct on
// card through the AddCounter replacements, as a cost and not an effect, so
// Vizier of Remedies and Vorinclex apply and Doubling Season does not, and
// the counters fire CounterAdded and enter the turn's log like any other
// placement.
func (g *Game) payCounterCost(controller PlayerController, payer PlayerID, card CardID, ct CounterType, n int) {
	if n = g.countersReplacedBy(controller, payer, CardEntity(card), ct, n, false); n > 0 {
		g.addCardCounters(controller, payer, card, card, ct, n)
	}
}

// counterAddedParams are the params this port reads on each mode; a trigger
// naming any other is skipped rather than fired without its condition (GO-7).
// The general ones (TriggerZones$, IsPresent$, OptionalDecider$, ...) are read
// by triggerEffectAPI.
var counterAddedParams = map[string][]string{
	"CounterAdded":     {"ValidCard", "ValidPlayer", "ValidSource", "CounterType", "CounterAmount"},
	"CounterAddedOnce": {"ValidCard", "ValidPlayer", "ValidSource", "CounterType"},
}

// counterAddedMatches appends every CounterAdded or CounterAddedOnce trigger
// (mode) that placer's placement of amount counters (the new total for
// CounterAdded) on object, a card or a player, fires. The placer is matched, never recorded:
// TriggerCounterAdded.java:97-98 and TriggerCounterAddedOnce.java:92-94 keep
// only Card and Player (and Amount) as triggering objects.
func (g *Game) counterAddedMatches(matches []Ability, mode string, placer PlayerID, object EntityID, ct CounterType, amount int) []Ability {
	var objects triggeredObjects
	if id, ok := object.AsCard(); ok {
		objects.card = id
	} else {
		objects.player, _ = object.AsPlayer()
	}
	for _, pid := range g.Players() {
		for _, host := range g.traitHosts(pid) {
			h := g.Card(host)
			if h.Def == nil {
				continue
			}
			for face := range h.triggerFaces {
				for _, t := range face.Triggers {
					if !strings.EqualFold(t.Name, mode) || !counterAddedTriggerReadable(t, mode) {
						continue
					}
					if !g.counterAddedTriggerMatches(t, h, host, placer, object, ct, amount) {
						continue
					}
					o := objects
					if mode == "CounterAddedOnce" {
						o.counts = triggerCounts{amount: amount, set: countAmount}
					}
					if sub, api, optional, ok := triggerEffectAPI(g, h, face.Amounts, t); ok {
						matches = append(matches, Ability{API: api, trigger: t, Source: host, Controller: h.Controller(), Params: sub, Amounts: face.Amounts, Optional: optional,
							triggered: face.objects(o), staticTrigger: isStaticTrigger(t)})
					}
				}
			}
		}
	}
	return matches
}

// counterAddedTriggerReadable reports whether every param of t is one this
// port evaluates for mode or one the general trigger gates read.
func counterAddedTriggerReadable(t *compile.Ability, mode string) bool {
	for _, p := range t.Params {
		switch strings.ToLower(p.Key) {
		case "mode", "execute", "triggerdescription", "secondary", "triggerzones", "chapter",
			"optionaldecider", "ispresent", "presentcompare", "presentzone", "presentplayer",
			"checksvar", "svarcompare", "playerturn", "phase", "opponentturn", "notplayerturn":
			continue
		}
		known := false
		for _, k := range counterAddedParams[mode] {
			known = known || strings.EqualFold(p.Key, k)
		}
		if !known {
			return false
		}
	}
	return true
}

// counterAddedTriggerMatches is TriggerCounterAdded.performTest
// (TriggerCounterAdded.java:57-90) and TriggerCounterAddedOnce's
// (TriggerCounterAddedOnce.java:58-89) over the params counterAddedParams
// lists. ValidSource$ is matched against the placer, AbilityKey.Source, a
// Player (Card.java:1805): every corpus value is "You". No placer (Java's
// null source) matches nothing, as matchesValidParam fails on null; a player
// spec matchesPlayerSpec cannot read leaves the trigger unfired rather than
// guessed.
func (g *Game) counterAddedTriggerMatches(t *compile.Ability, h *Card, host CardID, placer PlayerID, object EntityID, ct CounterType, amount int) bool {
	// matchesValidParam on the run's Card or Player, the other one null: a
	// ValidCard$ line never fires for a player's counters, a ValidPlayer$ one
	// never for a card's.
	id, isCard := object.AsCard()
	pid, isPlayer := object.AsPlayer()
	if vc, ok := t.Param("ValidCard"); ok && (!isCard || !Matches(g, g.Card(id), valid.Parse(vc), h.Controller(), host)) {
		return false
	}
	if vp, ok := t.Param("ValidPlayer"); ok {
		if !isPlayer {
			return false
		}
		if matched, recognized := matchesPlayerSpec(g, pid, h.Controller(), host, vp); !recognized || !matched {
			return false
		}
	}
	if vs, ok := t.Param("ValidSource"); ok {
		if placer == NoPlayer {
			return false
		}
		if matched, recognized := matchesPlayerSpec(g, placer, h.Controller(), host, vs); !recognized || !matched {
			return false
		}
	}
	if want, ok := t.Param("CounterType"); ok && !strings.EqualFold(want, string(ct)) {
		return false
	}
	if cmp, ok := t.Param("CounterAmount"); ok {
		// "EQ2": Expressions.compare of the new total against the operand.
		if len(cmp) < 3 {
			return false
		}
		operand, err := strconv.Atoi(cmp[2:])
		if err != nil || !compareOp(amount, cmp[:2], operand) {
			return false
		}
	}
	return true
}

// counterPlayerAddedAllMatches is Mode$ CounterPlayerAddedAll
// (TriggerCounterPlayerAddedAll.performTest) for placer's placement of n
// counters on object: ValidSource$ against the placer, ValidObject$ against
// the card or player. The trigger's Amount is the counters placed
// (setTriggeringObjects: the CounterMap's size), All Will Be One's "that
// much". A ValidObjectToSource$ line (every corpus one names a creature) does
// not fire for a player. No placer fires nothing (Optional.get() on an empty
// source cannot run).
func (g *Game) counterPlayerAddedAllMatches(placer PlayerID, object EntityID, n int) []Ability {
	if placer == NoPlayer {
		return nil
	}
	id, isCard := object.AsCard()
	pid, _ := object.AsPlayer()
	return g.scanTriggers([]string{"CounterPlayerAddedAll"}, func(h *Card, _ triggerFace, t *compile.Ability) (triggeredObjects, bool) {
		if !paramsResolvable(t, counterPlayerAddedAllParams) || !g.triggerPlayerMatches(h, t, "ValidSource", placer) {
			return triggeredObjects{}, false
		}
		counts := triggerCounts{amount: n, set: countAmount}
		if !isCard {
			if _, ok := t.Param("ValidObjectToSource"); ok || !g.triggerPlayerMatches(h, t, "ValidObject", pid) {
				return triggeredObjects{}, false
			}
			return triggeredObjects{player: pid, counts: counts}, true
		}
		c := g.Card(id)
		if !g.triggerCardMatches(h, t, "ValidObject", c) {
			return triggeredObjects{}, false
		}
		if spec, ok := t.Param("ValidObjectToSource"); ok && !Matches(g, c, valid.Parse(spec), placer, h.ID) {
			return triggeredObjects{}, false
		}
		return triggeredObjects{card: id, counts: counts}, true
	})
}

var (
	counterPlayerAddedAllParams = effectEventParams("validobject", "validsource", "validobjecttosource")
	counterAddedAllParams       = effectEventParams("valid", "validsource", "countertype")
	counterTypeAddedAllParams   = effectEventParams("validobject", "firsttime")
)

// countersAddedToThisTurn is Game.getCounterAddedThisTurn(null, c) > 0: some
// counter was put on the card by a player this turn.
func (g *Game) countersAddedToThisTurn(id CardID) bool {
	for i := range g.countersAddedThisTurn {
		if g.countersAddedThisTurn[i].card.ID == id {
			return true
		}
	}
	return false
}

// checkCounterBatchTriggers fires Mode$ CounterPlayerAddedAll,
// CounterAddedAll and CounterTypeAddedAll (GameEntityCounterTable.java:92-109,
// :170-175) for one placement of n counters of ct on card id. Java batches
// every counter one effect puts on all its targets into one table; this port
// fires per placement, a one-entry table (see quirks). A placement with no
// placer fires no CounterPlayerAddedAll (Optional.get() on an empty source
// cannot run). Counters on players fire CounterPlayerAddedAll from
// addPlayerCounters; CounterAddedAll and CounterTypeAddedAll read cards only,
// as every corpus Valid$/ValidObject$ of theirs names a card.
func (g *Game) checkCounterBatchTriggers(controller PlayerController, placer PlayerID, id CardID, ct CounterType, n int, first bool) {
	c := g.Card(id)
	matches := g.counterPlayerAddedAllMatches(placer, CardEntity(id), n)
	matches = append(matches, g.scanTriggers([]string{"CounterAddedAll"}, func(h *Card, _ triggerFace, t *compile.Ability) (triggeredObjects, bool) {
		if !paramsResolvable(t, counterAddedAllParams) || !g.triggerCardMatches(h, t, "Valid", c) {
			return triggeredObjects{}, false
		}
		if _, ok := t.Param("ValidSource"); ok && (placer == NoPlayer || !g.triggerPlayerMatches(h, t, "ValidSource", placer)) {
			return triggeredObjects{}, false
		}
		if want, ok := t.Param("CounterType"); ok && !strings.EqualFold(want, string(ct)) {
			return triggeredObjects{}, false
		}
		return triggeredObjects{card: id, counts: triggerCounts{amount: n, set: countAmount}}, true
	})...)
	matches = append(matches, g.scanTriggers([]string{"CounterTypeAddedAll"}, func(h *Card, _ triggerFace, t *compile.Ability) (triggeredObjects, bool) {
		if !paramsResolvable(t, counterTypeAddedAllParams) || !g.triggerCardMatches(h, t, "ValidObject", c) {
			return triggeredObjects{}, false
		}
		if _, ok := t.Param("FirstTime"); ok && !first {
			return triggeredObjects{}, false
		}
		return triggeredObjects{card: id}, true
	})...)
	g.pushTriggeredAbilities(controller, matches)
}
