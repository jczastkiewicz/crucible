// Continuous effects: CR 613, two layers deep so far. Layer 7b/7c's own
// power/toughness keys (SetPower$/SetToughness$/AddPower$/AddToughness$) are
// the single most common real corpus shape (2,192 of 2,426 real S:Mode$
// Continuous lines carrying one of these four keys, port-log/game-state.md's
// "Continuous effects" section); Layer 4's own type-changing keys (AddType$/
// RemoveType$, applyContinuousType below) are the next slice, both evaluated
// against the same blanket Affected$ valid-string.
//
// Ported from
// forge-game/src/main/java/forge/game/staticability/StaticAbilityContinuous.java's
// applyContinuousAbility/getAffectedCards.

package engine

import (
	"strconv"
	"strings"

	"github.com/jczastkiewicz/crucible/internal/carddb/compile"
	"github.com/jczastkiewicz/crucible/internal/cardtype"
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

// applyContinuousType recomputes every battlefield permanent's own Layer 4
// TypeMod effects from scratch, from every real Mode$ Continuous S: line
// currently in play -- applyContinuousPT's own reasoning applies identically
// here: Java's own applyContinuousAbility runs fresh from
// GameAction.checkStateEffects every state-based-action pass, not stored and
// incrementally updated, so a type-granting effect (an anthem-shaped
// "creatures you control are Zombies") has to reach a creature that enters
// after it and stop the instant it itself leaves.
func applyContinuousType(g *Game) {
	for _, pid := range g.Players() {
		for _, id := range g.Zone(Battlefield, pid).Cards() {
			g.Card(id).TypeMod.Clear()
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
					applyOneContinuousType(g, h, s)
				}
			}
		}
	}
}

// applyOneContinuousType is applyOneContinuousPT's own Layer 4 counterpart:
// s applies to every battlefield permanent its own Affected$ valid-string
// matches, if s is a Mode$ Continuous line naming AddType$ and/or RemoveType$
// in the one shape this slice can resolve -- a plain, space-and-ampersand
// (" & ") separated list of literal type words, no dynamic value and no
// bulk-removal flag.
//
// A whole line is skipped, not applied partially, the instant it carries
// anything past that shape (game-state.md's "Continuous effects" section has
// the corpus counts):
//   - Condition$/AffectedDefined$/AffectedZone$/CharacteristicDefining$ --
//     applyOneContinuousPT's own four skip reasons, identical here since all
//     four are properties of the static ability itself, not of which layer
//     it happens to write to.
//   - ChosenType$/ChosenType2$/ImprintedCreatureType$/AllBasicLandType$/
//     AllNonBasicLandType$ as an AddType$ or RemoveType$ token (29 of 256
//     real AddType$ lines) -- each needs a runtime value (a chosen type, an
//     imprinted card's own creature types, the basic-land-type enum) this
//     port has no evaluator for.
//   - RemoveSuperTypes$/RemoveCardTypes$/RemoveSubTypes$/RemoveLandTypes$/
//     RemoveCreatureTypes$/RemoveArtifactTypes$/RemoveEnchantmentTypes$ (62
//     of 284 real AddType$/RemoveType$ lines) -- a bulk "wipe this whole
//     category first" flag most often paired with AddType$ in a real "becomes
//     a Turtle" shape (StaticAbilityContinuous.java:425-448); applying AddType$
//     alone without the wipe would leave the card BOTH its old and new
//     creature types, an actively wrong answer worse than the coverage gap of
//     skipping the whole line (the same reasoning Intimidate's own doc
//     comment, staticability.go, already gives for a property this port would
//     otherwise get backwards).
//   - AddAllCreatureTypes$ (8) -- every creature type in the game, an enum
//     this port's cardtype.Registry is not plumbed into the engine to read
//     from a static-ability effect yet (ParseToken's own doc comment,
//     cardtype.go).
//
// 173 of 256 real AddType$ lines and all 28 real RemoveType$ lines (173+28 of
// 284, game-state.md) carry none of the above and resolve here.
func applyOneContinuousType(g *Game, host *Card, s *compile.Ability) {
	if !strings.EqualFold(s.Name, "Continuous") {
		return
	}
	for _, key := range [...]string{
		"Condition", "AffectedDefined", "AffectedZone", "CharacteristicDefining",
		"AddAllCreatureTypes",
		"RemoveSuperTypes", "RemoveCardTypes", "RemoveSubTypes", "RemoveLandTypes",
		"RemoveCreatureTypes", "RemoveArtifactTypes", "RemoveEnchantmentTypes",
	} {
		if _, ok := s.Param(key); ok {
			return
		}
	}
	addTypes, hasAdd := typeTokens(s, "AddType")
	removeTypes, hasRemove := typeTokens(s, "RemoveType")
	if !hasAdd && !hasRemove {
		return
	}
	affected, ok := s.Param("Affected")
	if !ok {
		return
	}

	spec := valid.Parse(affected)
	for _, pid := range g.Players() {
		for _, id := range g.Zone(Battlefield, pid).Cards() {
			if !Matches(g, g.Card(id), spec, host.Controller, host.ID) {
				continue
			}
			g.Card(id).TypeMod.Add(TypeEffect{Timestamp: host.Timestamp, AddTypes: addTypes, RemoveTypes: removeTypes})
		}
	}
}

// typeTokens reads key (AddType$ or RemoveType$) as its own " & "-separated
// list of literal type words, each classified by cardtype.ParseToken and
// unioned together -- the fragment TypeEffect carries. false, along with a
// dynamic value (ChosenType and the rest, applyOneContinuousType's own list)
// mixed anywhere into the list, since a token this cannot resolve makes the
// whole line's own Add/Remove set wrong, not just incomplete (the same
// whole-line skip its own doc comment explains).
func typeTokens(s *compile.Ability, key string) (cardtype.Line, bool) {
	v, ok := s.Param(key)
	if !ok {
		return cardtype.Line{}, false
	}
	var out cardtype.Line
	for _, word := range strings.Split(v, " & ") {
		switch word {
		case "ChosenType", "ChosenType2", "ImprintedCreatureType", "AllBasicLandType", "AllNonBasicLandType":
			return cardtype.Line{}, false
		}
		out = out.Union(cardtype.ParseToken(word))
	}
	return out, true
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
