// Mode$ BecomesCrewed and Mode$ UntapAll: the triggers a resolved Crew ability
// and an untapping fire once for the whole batch (Card.becomesCrewed,
// Card.java:21-28; Untap.java:170-173), unlike Crewed, which runs per creature
// as the ability is put on the stack (crewedMatches).

//enginelint:allow id zone card game valid ability event amount parts trigger statictrigger staticability control

package engine

import (
	"strconv"

	"github.com/jczastkiewicz/crucible/internal/carddb/compile"
	"github.com/jczastkiewicz/crucible/internal/valid"
)

// becomesCrewedParams are the params a BecomesCrewed trigger may carry that
// are evaluated. ValidCrew$ is left out: the run's Crew value is the whole
// list of tapped creatures, which no corpus line filters.
var becomesCrewedParams = map[string]bool{
	"mode": true, "execute": true, "triggerdescription": true, "triggerzones": true, "optionaldecider": true,
	"secondary": true, "validcard": true, "firsttimecrewed": true, "validcrewamount": true,
	"playerturn": true, "phase": true,
}

// becomesCrewed is AnimateEffect.java:203-205's Card.becomesCrewed for a
// resolved Crew ability a: the host's crew count for the turn goes up, then
// BecomesCrewed fires once, FirstTimeCrewed$ meaning that count is 1 and
// ValidCrewAmount$ the number of creatures tapped for the ability.
func (g *Game) becomesCrewed(controller PlayerController, a *Ability) {
	host := a.Source
	g.crewedThisTurn = append(g.crewedThisTurn, host)
	times := 0
	for _, id := range g.crewedThisTurn {
		if id == host {
			times++
		}
	}
	crew := len(a.paid.tapped)
	matches := g.scanTriggers([]string{"BecomesCrewed"}, func(h *Card, _ triggerFace, t *compile.Ability) (triggeredObjects, bool) {
		if !paramsResolvable(t, becomesCrewedParams) || !g.triggerCardMatches(h, t, "ValidCard", g.Card(host)) {
			return triggeredObjects{}, false
		}
		if _, ok := t.Param("FirstTimeCrewed"); ok && times != 1 {
			return triggeredObjects{}, false
		}
		if v, ok := t.Param("ValidCrewAmount"); ok {
			if n, err := strconv.Atoi(v); err != nil || n != crew {
				return triggeredObjects{}, false
			}
		}
		return triggeredObjects{card: host}, true
	})
	g.pushTriggeredAbilities(controller, matches)
}

// untapAllParams are the params an UntapAll trigger may carry that are
// evaluated.
var untapAllParams = map[string]bool{
	"mode": true, "execute": true, "triggerdescription": true, "triggerzones": true, "optionaldecider": true,
	"secondary": true, "validcards": true, "validplayer": true, "playerturn": true, "phase": true,
}

// untapAllMatches is Untap.java:170-173's UntapAll run for the permanents pid's
// untap step untapped: one run per trigger whose ValidCards$ keeps any of them
// (TriggerUntapAll.filteredMap: a line with no ValidCards$ keeps none,
// TriggerUntapAll.java:55-58). The effects that untap (UntapAll,
// UntapEffect, costs) fire it in Java too; here only the untap step does.
func (g *Game) untapAllMatches(pid PlayerID, untapped []CardID) []Ability {
	if len(untapped) == 0 {
		return nil
	}
	return g.scanTriggers([]string{"UntapAll"}, func(h *Card, _ triggerFace, t *compile.Ability) (triggeredObjects, bool) {
		if !paramsResolvable(t, untapAllParams) || !g.triggerPlayerMatches(h, t, "ValidPlayer", pid) {
			return triggeredObjects{}, false
		}
		spec, ok := t.Param("ValidCards")
		if !ok {
			return triggeredObjects{}, false
		}
		parsed := valid.Parse(spec)
		var kept []CardID
		for _, id := range untapped {
			if Matches(g, g.Card(id), parsed, h.Controller(), h.ID) {
				kept = append(kept, id)
			}
		}
		if len(kept) == 0 {
			return triggeredObjects{}, false
		}
		return triggeredObjects{player: pid, cards: kept, counts: triggerCounts{amount: len(kept), set: countAmount}}, true
	})
}

// becomesSaddledParams are the params a BecomesSaddled trigger may carry that
// are evaluated.
var becomesSaddledParams = map[string]bool{
	"mode": true, "execute": true, "triggerdescription": true, "triggerzones": true, "optionaldecider": true,
	"secondary": true, "validsaddled": true, "firsttimesaddled": true, "playerturn": true, "phase": true,
}

// becomesSaddled is AlterAttributeEffect.java:104-113's BecomesSaddled run for
// the Mount host: FirstTimeSaddled$ holds when it has been saddled once this
// turn (Card.getTimesSaddledThisTurn, already counted).
func (g *Game) becomesSaddled(controller PlayerController, host CardID, _ []CardID) {
	times := g.Card(host).timesSaddled
	matches := g.scanTriggers([]string{"BecomesSaddled"}, func(h *Card, _ triggerFace, t *compile.Ability) (triggeredObjects, bool) {
		if !paramsResolvable(t, becomesSaddledParams) || !g.triggerCardMatches(h, t, "ValidSaddled", g.Card(host)) {
			return triggeredObjects{}, false
		}
		if _, ok := t.Param("FirstTimeSaddled"); ok && times != 1 {
			return triggeredObjects{}, false
		}
		return triggeredObjects{card: host}, true
	})
	g.pushTriggeredAbilities(controller, matches)
}
