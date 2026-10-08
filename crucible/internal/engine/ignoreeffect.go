// IgnoreEffectCost$ (Leonin Arbiter): "any player may pay {2} for that player
// to ignore this effect until end of turn."
//
// Ported from forge-game/src/main/java/forge/game/staticability/
// StaticAbilityContinuous.java (buildIgnoreEffectAbility, :948-985; the
// affected-player filter, :1023), StaticAbility.java:517-545 and StaticEffect.java
// :184-186. Java builds an AbilityStatic on the static's host when the static
// applies; compile builds the same ability once as a sub of the static
// (compile.ignoreEffectSub), the engine grants it to the host each pass
// (grantIgnoreEffect) and InternalIgnoreEffect resolves it.

package engine

//enginelint:allow id game card player ability continuouslayers zone valid control

import (
	"strings"

	"github.com/jczastkiewicz/crucible/internal/carddb/compile"
	"github.com/jczastkiewicz/crucible/internal/expr"
)

// ignoreEffectSubOf is the InternalIgnoreEffect ability compile attached to a
// static that carries IgnoreEffectCost$, or nil.
func ignoreEffectSubOf(s *compile.Ability) *compile.Ability {
	for _, sub := range s.Subs {
		if strings.EqualFold(sub.Key, "IgnoreEffectCost") {
			return sub.Ability
		}
	}
	return nil
}

// grantIgnoreEffect gives host the ignore-effect ability of a static of its
// own that carries IgnoreEffectCost$ and applies this pass
// (StaticAbilityContinuous.java:491-494: `addChangedCardTraits` on the host,
// not on the affected cards). Only a Mode$ Continuous static reads the ignore
// set (applyOneContinuousKeyword, applyOneContinuousRules); other modes keep
// their ability granted but the set is not consulted for them yet.
func grantIgnoreEffect(g *Game, host *Card, amounts map[string]expr.Amount, s *compile.Ability) {
	sub := ignoreEffectSubOf(s)
	if sub == nil || !layerStaticApplies(g, host, amounts, s) {
		return
	}
	host.traitGrants = append(append([]traitGrant(nil), host.traitGrants...), traitGrant{abilities: []*compile.Ability{sub}, amounts: amounts})
}

// ignoreStatic finds the static of host whose ignore-effect ability is ab.
func ignoreStatic(host *Card, ab *compile.Ability) *compile.Ability {
	for _, face := range host.traitFaces() {
		for _, s := range face.Statics {
			if ignoreEffectSubOf(s) == ab {
				return s
			}
		}
	}
	return nil
}

// ignoreActivatorValid is the canPlay of the ability buildIgnoreEffectAbility
// makes (:963-966): the host is in play and the activator is one of the
// static's affected players (the controllers of its affected cards, which Java
// adds, are not modelled: only Mode$ Continuous statics read the set). The
// ability has no timing restriction.
func (g *Game) ignoreActivatorValid(pid PlayerID, host *Card, ab *compile.Ability) bool {
	s := ignoreStatic(host, ab)
	if s == nil || host.Zone != Battlefield || !strings.EqualFold(s.Name, "Continuous") {
		return false
	}
	spec, ok := s.Param("Affected")
	if !ok {
		return false
	}
	matched, recognized := matchesPlayerSpec(g, pid, host.Controller(), host.ID, spec)
	return recognized && matched
}

// ignoredEffect is one resolved ignore-effect: players who ignore the static
// until the turn ends or its host leaves play (StaticAbility.
// getIgnoreEffectPlayers; the removal commands :976-984).
type ignoredEffect struct {
	host    CardID
	stamp   uint64
	static  *compile.Ability
	players []PlayerID
}

// cloneIgnores is Game.Clone's copy: the player lists must not alias.
func cloneIgnores(src []ignoredEffect) []ignoredEffect {
	if src == nil {
		return nil
	}
	out := make([]ignoredEffect, len(src))
	for i, e := range src {
		e.players = append([]PlayerID(nil), e.players...)
		out[i] = e
	}
	return out
}

// playerIgnores is `stAb.getIgnoreEffectPlayers().contains(pid)`
// (StaticAbilityContinuous.java:1023 removes them from the affected players).
func (g *Game) playerIgnores(pid PlayerID, host *Card, s *compile.Ability) bool {
	for _, e := range g.ignores {
		if e.static != s || e.host != host.ID || e.stamp != host.zoneStamp {
			continue
		}
		for _, p := range e.players {
			if p == pid {
				return true
			}
		}
	}
	return false
}

// internalIgnoreEffectEffect resolves the ability buildIgnoreEffectAbility makes:
// the activating player ignores the static for the rest of the turn.
type internalIgnoreEffectEffect struct{}

func (internalIgnoreEffectEffect) Resolve(g *Game, a *Ability, _ PlayerController) error {
	host := g.Card(a.Source)
	s := ignoreStatic(host, a.Params)
	if s == nil {
		return nil
	}
	for i := range g.ignores {
		e := &g.ignores[i]
		if e.static == s && e.host == host.ID && e.stamp == host.zoneStamp {
			for _, p := range e.players {
				if p == a.Controller {
					return nil
				}
			}
			e.players = append(append([]PlayerID(nil), e.players...), a.Controller)
			return nil
		}
	}
	g.ignores = append(g.ignores, ignoredEffect{host: host.ID, stamp: host.zoneStamp, static: s, players: []PlayerID{a.Controller}})
	return nil
}
