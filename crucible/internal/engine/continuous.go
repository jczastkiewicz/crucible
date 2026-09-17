// Continuous effects: CR 613, trimmed to Layer 7b/7c's own power/toughness
// keys (SetPower$/SetToughness$/AddPower$/AddToughness$) evaluated against
// a blanket Affected$ valid-string -- the single most common real corpus
// shape (2,192 of 2,426 real S:Mode$ Continuous lines carrying one of these
// four keys, port-log/game-state.md's "Continuous effects" section).
//
// Ported from
// forge-game/src/main/java/forge/game/staticability/StaticAbilityContinuous.java's
// applyContinuousAbility/getAffectedCards.

package engine

import (
	"strconv"
	"strings"

	"github.com/jczastkiewicz/crucible/internal/carddb/compile"
	"github.com/jczastkiewicz/crucible/internal/valid"
)

// applyContinuousPT recomputes every battlefield permanent's own Layer
// 7b/7c PTEffects from scratch, from every real Mode$ Continuous S: line
// currently in play. CR 613's own continuous effects are not stored and
// incrementally updated the way a resolved spell's own damage or a counter
// is -- Java's own applyContinuousAbility runs fresh from
// GameAction.checkStateEffects every state-based-action pass, which is why
// this is called from CheckStateBasedActions (action.go) rather than from
// wherever a permanent enters or leaves: an anthem effect has to apply to a
// creature that enters AFTER it, and stop applying the instant the anthem
// itself leaves, neither of which a one-time push at either card's own
// entry could give it.
//
// Every battlefield card's own PT.effects is cleared first, then rebuilt --
// safe today because nothing else this port can build yet ever adds a
// PTEffect: a real "+3/+3 until end of turn" pump spell would need its own
// duration-scoped bucket this clear would not touch (game-state.md's "Not
// ported yet"), so recomputing everything from Mode$ Continuous statics
// alone is exactly correct until one exists, not an approximation that
// happens to work today.
func applyContinuousPT(g *Game) {
	for _, pid := range g.Players() {
		for _, id := range g.Zone(Battlefield, pid).Cards() {
			g.Card(id).PT.Clear()
		}
	}
	for _, pid := range g.Players() {
		for _, host := range g.Zone(Battlefield, pid).Cards() {
			h := g.Card(host)
			if h.Def == nil {
				continue
			}
			for _, face := range h.Def.Faces {
				for _, s := range face.Statics {
					applyOneContinuousPT(g, h, s)
				}
			}
		}
	}
}

// applyOneContinuousPT applies s to every battlefield permanent its own
// Affected$ valid-string matches, if s is a Mode$ Continuous line this slice
// can resolve.
//
// Not resolved, each for a specific reason (game-state.md's "Continuous
// effects" section has the corpus counts behind every number below):
//   - Condition$ (116 of 2,426) -- a generic runtime gate ("during your
//     turn," and the like) StaticAbility.java's own checkConditions
//     evaluates for every static-ability mode; this port has no equivalent
//     for any mode yet, so a line carrying it is skipped rather than
//     treated as always-true.
//   - AffectedDefined$/AffectedZone$ (0 and 24) -- a targeted or
//     Remembered-driven affected set (AbilityUtils.getDefinedCards) rather
//     than a blanket valid-string match against the whole battlefield.
//   - CharacteristicDefining$ True (265) -- Layer 7a: the ability describes
//     the host's OWN power/toughness (a "*/*" creature), almost always via
//     a Count$ SVar this port has no evaluator for (the identical gap
//     compareMatches, valid.go, already documents) -- skipped as a whole
//     rather than only for the SVar-driven cases, since a plain-integer
//     CharacteristicDefining line is not a real corpus shape worth a
//     separate code path for.
//   - A non-numeric AddPower$/AddToughness$/SetPower$/SetToughness$ (X, Y,
//     Z, AffectedX, a named SVar) -- needs AbilityUtils.calculateAmount,
//     which needs an ability-context evaluator internal/expr does not have.
//     Skipped per missing dimension (ptParam, below), not per whole line:
//     a real corpus line naming both a resolvable and an unresolvable
//     dimension together is not a shape worth losing the resolvable half
//     over.
func applyOneContinuousPT(g *Game, host *Card, s *compile.Ability) {
	if !strings.EqualFold(s.Name, "Continuous") {
		return
	}
	for _, key := range [...]string{"Condition", "AffectedDefined", "AffectedZone", "CharacteristicDefining"} {
		if _, ok := s.Param(key); ok {
			return
		}
	}
	affected, ok := s.Param("Affected")
	if !ok {
		return
	}
	addP, hasAddP := ptParam(s, "AddPower")
	addT, hasAddT := ptParam(s, "AddToughness")
	setP, hasSetP := ptParam(s, "SetPower")
	setT, hasSetT := ptParam(s, "SetToughness")
	if !hasAddP && !hasAddT && !hasSetP && !hasSetT {
		return
	}

	spec := valid.Parse(affected)
	for _, pid := range g.Players() {
		for _, id := range g.Zone(Battlefield, pid).Cards() {
			if !Matches(g, g.Card(id), spec, host.Controller, host.ID) {
				continue
			}
			c := g.Card(id)
			if hasSetP || hasSetT {
				c.PT.Add(PTEffect{
					Layer: LayerSetPT, Timestamp: host.Timestamp,
					Power: setP, Toughness: setT,
					HasPower: hasSetP, HasToughness: hasSetT,
				})
			}
			if hasAddP || hasAddT {
				c.PT.Add(PTEffect{Layer: LayerModifyPT, Timestamp: host.Timestamp, Power: addP, Toughness: addT})
			}
		}
	}
}

// ptParam reads key as a plain base-10 integer (optionally negative) --
// AddPower$/AddToughness$/SetPower$/SetToughness$'s own corpus-frequent
// shape. Reports false for a missing key or a non-numeric one (X, Y, Z,
// AffectedX, a named SVar), the same "not a plain integer, coverage gap
// rather than a wrong answer" contract compareMatches (valid.go) already
// documents.
func ptParam(s *compile.Ability, key string) (int, bool) {
	v, ok := s.Param(key)
	if !ok {
		return 0, false
	}
	n, err := strconv.Atoi(v)
	return n, err == nil
}
