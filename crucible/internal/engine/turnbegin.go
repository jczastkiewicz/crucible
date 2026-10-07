// Mode$ TurnBegin: "Turn Begin isn't a 'real' trigger, but is useful for
// Advanced Scripting Techniques" (TriggerTurnBegin.java). Forge runs it once
// per turn, as the cleanup step of the previous turn hands the turn to the next
// player (PhaseHandler.java:522-524), so it fires even when the turn's phases
// are skipped. Every corpus line is a Static$ True reset (Krovikan Vampire,
// Cobra Trap, Arboria, Monitor Monitor, ...).

package engine

//enginelint:allow id card game ability control trigger statictrigger valid

import "strings"

// checkTurnBeginTriggers fires the Mode$ TurnBegin triggers of every trait
// host for player, whose turn it now is. ValidPlayer$ (absent: any player) is
// read through matchesPlayerSpec; a spec it cannot read skips the line.
func (g *Game) checkTurnBeginTriggers(controller PlayerController, player PlayerID) {
	var matches []Ability
	for _, pid := range g.Players() {
		for _, host := range g.traitHosts(pid) {
			h := g.Card(host)
			if h.Def == nil {
				continue
			}
			for face := range h.triggerFaces {
				for _, t := range face.Triggers {
					if !strings.EqualFold(t.Name, "TurnBegin") {
						continue
					}
					if spec, ok := t.Param("ValidPlayer"); ok {
						if matched, recognized := matchesPlayerSpec(g, player, h.Controller(), host, spec); !recognized || !matched {
							continue
						}
					}
					if sub, api, optional, ok := triggerEffectAPI(g, h, face.Amounts, t); ok {
						matches = append(matches, Ability{API: api, Source: host, Controller: h.Controller(), Params: sub, Amounts: face.Amounts, Optional: optional, triggered: face.objects(triggeredObjects{player: player}), staticTrigger: isStaticTrigger(t)})
					}
				}
			}
		}
	}
	g.pushTriggeredAbilities(controller, matches)
}
