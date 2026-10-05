// CR 616.1: when more than one replacement effect could apply to an event,
// the affected player chooses which applies first, and the rest are looked at
// again against what is left of the event (CR 616.1f). Ported from
// ReplacementHandler.run (ReplacementHandler.java:169-290): collect the
// candidates, ask the decider when there is a real choice, apply the chosen
// one and mark it applied, then either stop (the event was replaced) or
// collect again with the applied ones excluded (the event was updated or the
// choice declined itself).
//
// Java runs this once per ReplacementLayer; the four dispatches that use it
// (Draw, GainLife, DamageDone to a card and to a player) are all
// ReplacementLayer.Other, whose CantHappen siblings (the Prevent$ True
// shapes) run before it in their own functions.

package engine

import (
	"fmt"
	"strings"

	"github.com/jczastkiewicz/crucible/internal/carddb/compile"
	"github.com/jczastkiewicz/crucible/internal/expr"
)

// replacementCandidate is one replacement effect that could apply to the
// event being run. apply edits the event the collecting closure owns and
// reports what it did: Replaced ends the event, Updated lets it go on with
// new values, NotReplaced (a declined or unresolvable shape) leaves it as it
// was.
type replacementCandidate struct {
	host  *Card
	rule  *compile.Ability
	apply func() replacementResult
}

// appliedReplacement is Java's ReplacementEffect.hasRun: a replacement
// effect applies at most once to one event, so a candidate already chosen is
// not offered again after the event was updated.
type appliedReplacement struct {
	host CardID
	rule *compile.Ability
}

// runReplacements runs the CR 616 loop for an event whose Affected is
// decider (the affected player, or the affected permanent's controller).
// collect lists the replacement effects that could apply to the event as it
// stands -- it is called again after every applied effect, since an updated
// amount can change which lines match -- in play order. It returns the
// event's overall outcome: Replaced if some effect replaced it, Updated if
// effects only resized it, NotReplaced if none applied.
func (g *Game) runReplacements(controller PlayerController, decider PlayerID, collect func() []replacementCandidate) replacementResult {
	var done []appliedReplacement
	outcome := replacementNotReplaced
	for {
		var cands []replacementCandidate
		for _, c := range collect() {
			if !appliedBefore(done, c) {
				cands = append(cands, c)
			}
		}
		if len(cands) == 0 {
			return outcome
		}
		chosen := cands[g.chooseReplacement(controller, decider, cands)]
		done = append(done, appliedReplacement{host: chosen.host.ID, rule: chosen.rule})
		switch chosen.apply() {
		case replacementReplaced:
			return replacementReplaced
		case replacementUpdated:
			outcome = replacementUpdated
		}
	}
}

func appliedBefore(done []appliedReplacement, c replacementCandidate) bool {
	for _, d := range done {
		if d.host == c.host.ID && d.rule == c.rule {
			return true
		}
	}
	return false
}

// chooseReplacement is PlayerController.chooseSingleReplacementEffect's
// call: the index of the candidate to apply. A lone candidate, or several
// that all read the same (one permanent, one Description$ -- Java compares
// ReplacementEffect.toString, host id included), is taken without asking. An
// answer out of range is a controller bug, recorded (GO-7) and answered with
// the first candidate.
func (g *Game) chooseReplacement(controller PlayerController, decider PlayerID, cands []replacementCandidate) int {
	if len(cands) == 1 || controller == nil {
		return 0
	}
	opts := make([]ReplacementOption, len(cands))
	same := true
	for i, c := range cands {
		desc, _ := c.rule.Param("Description")
		opts[i] = ReplacementOption{Host: c.host.ID, Description: strings.ReplaceAll(desc, "CARDNAME", c.host.Def.Name)}
		if opts[i] != opts[0] {
			same = false
		}
	}
	if same {
		return 0
	}
	i := controller.ChooseReplacementEffect(g, decider, opts)
	if i < 0 || i >= len(cands) {
		g.recordPendingError(fmt.Errorf("engine: ChooseReplacementEffect answered %d of %d candidates", i, len(cands)))
		return 0
	}
	return i
}

// eachReplacementRule calls fn for every replacement line of every card on
// the battlefield or in the command zone, in play order: players, then
// zones, then cards, then each card's faces and the lines in them.
// Host eligibility and the line's own Event$ are fn's to check.
func (g *Game) eachReplacementRule(fn func(h *Card, zone ZoneType, amounts map[string]expr.Amount, r *compile.Ability)) {
	for _, pid := range g.Players() {
		for _, z := range replacementZones {
			for _, host := range g.Zone(z, pid).Cards() {
				h := g.Card(host)
				if h.Def == nil {
					continue
				}
				for _, face := range h.liveTraitFaces() {
					for _, r := range face.Replacements {
						fn(h, z, face.Amounts, r)
					}
				}
			}
		}
	}
}
