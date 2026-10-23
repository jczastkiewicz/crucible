// Mode$ Always, CR 603.8's state trigger: "when you control no Islands,
// sacrifice this", "when you have 30 or more life, flip it" (62 real corpus
// lines). TriggerAlways.performTest is true; the line's IsPresent$,
// LifeTotal$ and CheckSVar$ conditions are its only gate, read by
// triggerCommonRequirementsMet. It is checked after every state-based action
// check (GameAction.checkStaticAbilities, :1244) and skipped while the same
// trigger is already on the stack (TriggerHandler.canRunTrigger, :425), so a
// state that stays true fires once until that ability has resolved.
//
// Ported from forge-game/src/main/java/forge/game/trigger/TriggerAlways.java
// and TriggerHandler.runStateTrigger (:280). ResolvingCheck$ names the one
// condition an Always ability re-checks as it resolves
// (WrappedAbility.java:421-430): Hidden Predators and the Opal Avenger
// family's "if CARDNAME is an enchantment". Java does not re-check the
// others for Always (:405).

package engine

//enginelint:allow id card game ability control zone condition parts trigger statictrigger

import (
	"strings"

	"github.com/jczastkiewicz/crucible/internal/carddb/compile"
)

// checkAlwaysTriggers puts every Mode$ Always trigger whose condition holds
// and that is not already waiting on the stack onto the stack.
func (g *Game) checkAlwaysTriggers(controller PlayerController) {
	var matches []Ability
	for _, pid := range g.Players() {
		for _, z := range phaseTriggerZones {
			for _, host := range g.Zone(z, pid).Cards() {
				h := g.Card(host)
				if h.Def == nil {
					continue
				}
				for face := range h.triggerFaces {
					for _, t := range face.Triggers {
						if !strings.EqualFold(t.Name, "Always") || !phaseTriggerZoneMatches(h, t, z) {
							continue
						}
						sub, api, optional, ok := triggerEffectAPI(g, h, face.Amounts, t)
						if !ok || g.alwaysOnStack(host, sub) {
							continue
						}
						m := Ability{API: api, Source: host, Controller: h.Controller(), Params: sub, Amounts: face.Amounts, Optional: optional, staticTrigger: isStaticTrigger(t)}
						if key, has := t.Param("ResolvingCheck"); has {
							m.recheck = withOnlyParam(t, key)
						}
						matches = append(matches, m)
					}
				}
			}
		}
	}
	g.pushTriggeredAbilities(controller, matches)
}

// alwaysOnStack reports whether host's trigger executing sub already waits on
// the stack.
func (g *Game) alwaysOnStack(host CardID, sub *compile.Ability) bool {
	for i := range g.stack {
		if s := &g.stack[i]; s.isTrigger && s.Source == host && s.Params == sub {
			return true
		}
	}
	return false
}

// withOnlyParam is t cut down to its one param key: what
// WrappedAbility.resolve hands meetsCommonRequirements to check again.
func withOnlyParam(t *compile.Ability, key string) *compile.Ability {
	cut := *t
	cut.Params = nil
	for _, p := range t.Params {
		if strings.EqualFold(p.Key, key) {
			cut.Params = append(cut.Params, p)
		}
	}
	return &cut
}

// recheckFails reports whether a's ResolvingCheck$ condition no longer holds
// as it resolves.
func (g *Game) recheckFails(a *Ability) bool {
	return a.recheck != nil && !triggerCommonRequirementsMet(g, g.Card(a.Source), a.Amounts, a.recheck)
}
