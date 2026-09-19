// Continuous effects: CR 613, four layers deep so far. Layer 7b/7c's own
// power/toughness keys (SetPower$/SetToughness$/AddPower$/AddToughness$) are
// the single most common real corpus shape (2,192 of 2,426 real S:Mode$
// Continuous lines carrying one of these four keys, port-log/game-state.md's
// "Continuous effects" section); Layer 4's own type-changing keys (AddType$/
// RemoveType$, applyContinuousType), Layer 5's own color-changing keys
// (AddColor$/SetColor$, applyContinuousColor) and Layer 6's own
// ability-granting key (AddKeyword$, applyContinuousKeyword below -- the
// single largest real slice of all four, 1,556 of 1,857 real lines) are the
// next three, all four evaluated against the same blanket Affected$
// valid-string.
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
	"github.com/jczastkiewicz/crucible/internal/expr"
	"github.com/jczastkiewicz/crucible/internal/mana"
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
					applyOneContinuousPT(g, h, face.Amounts, s)
				}
			}
		}
	}
}

// applyOneContinuousPT applies s to every battlefield permanent its own
// Affected$ valid-string matches, if s is a Mode$ Continuous line this slice
// can resolve -- or, when s is CharacteristicDefining$ (Layer 7a), computes
// host's own power/toughness and applies it to host alone
// (applyOneCharacteristicDefiningPT, below).
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
//   - A non-numeric, non-resolvable AddPower$/AddToughness$/SetPower$/
//     SetToughness$ -- resolveAmount (amount.go) now evaluates a named SVar
//     whose own body is a Count$Valid* expression (ptParam, below); a plain
//     integer resolves as it always did, and only a genuinely unresolvable
//     value (xPaid, an operator suffix, ChosenNumber, ...) is skipped, per
//     missing dimension rather than per whole line -- a real corpus line
//     naming both a resolvable and an unresolvable dimension together is
//     not a shape worth losing the resolvable half over.
func applyOneContinuousPT(g *Game, host *Card, amounts map[string]expr.Amount, s *compile.Ability) {
	if !strings.EqualFold(s.Name, "Continuous") {
		return
	}
	for _, key := range [...]string{"Condition", "AffectedDefined", "AffectedZone"} {
		if _, ok := s.Param(key); ok {
			return
		}
	}
	if _, ok := s.Param("CharacteristicDefining"); ok {
		applyOneCharacteristicDefiningPT(g, host, amounts, s)
		return
	}
	affected, ok := s.Param("Affected")
	if !ok {
		return
	}
	addP, hasAddP := ptParam(g, amounts, host, s, "AddPower")
	addT, hasAddT := ptParam(g, amounts, host, s, "AddToughness")
	setP, hasSetP := ptParam(g, amounts, host, s, "SetPower")
	setT, hasSetT := ptParam(g, amounts, host, s, "SetToughness")
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

// applyOneCharacteristicDefiningPT is Layer 7a: a characteristic-defining
// ability's own SetPower$/SetToughness$ describes what host's power/
// toughness IS, not an anthem effect reaching other permanents --
// StaticAbilityContinuous.getAffectedCards' own CharacteristicDefining
// branch hardcodes the affected set to `new CardCollection(hostCard)`
// regardless of any Affected$ a real corpus line happens to also carry
// (revenant.txt's own "Affected$ Card.Self," redundant with what Java
// already does unconditionally) -- so this reads no Affected$ param at all,
// unlike every other applyOneContinuous* sibling.
//
// AddPower$/AddToughness$ are not read here: CR 613.3's own "characteristic-
// defining ability... functions in the layer the appropriate
// characteristic-setting ability would normally apply" means a CDA always
// SETS the base value it defines, never adds to one -- no real corpus
// CharacteristicDefining line pairs SetPower$/SetToughness$ with an
// Add-shaped key.
//
// ExcludeZone$ (1 real line among 264 CharacteristicDefining$ True cards) --
// skip host entirely while it sits in one of the named zones -- is not
// resolved: a single real line is not a shape worth a separate zone check
// for, and applyContinuousPT's own battlefield-only walk means host is
// always on the one zone this port could check anyway.
func applyOneCharacteristicDefiningPT(g *Game, host *Card, amounts map[string]expr.Amount, s *compile.Ability) {
	if _, ok := s.Param("ExcludeZone"); ok {
		return
	}
	setP, hasSetP := ptParam(g, amounts, host, s, "SetPower")
	setT, hasSetT := ptParam(g, amounts, host, s, "SetToughness")
	if !hasSetP && !hasSetT {
		return
	}
	host.PT.Add(PTEffect{
		Layer: LayerCharacteristic, Timestamp: host.Timestamp,
		Power: setP, Toughness: setT,
		HasPower: hasSetP, HasToughness: hasSetT,
	})
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

// applyContinuousColor recomputes every battlefield permanent's own Layer 5
// ColorMod effects from scratch, from every real Mode$ Continuous S: line
// currently in play -- applyContinuousPT's/applyContinuousType's own
// reasoning applies identically here.
func applyContinuousColor(g *Game) {
	for _, pid := range g.Players() {
		for _, id := range g.Zone(Battlefield, pid).Cards() {
			g.Card(id).ColorMod.Clear()
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
					applyOneContinuousColor(g, h, s)
				}
			}
		}
	}
}

// applyOneContinuousColor is applyOneContinuousPT's/applyOneContinuousType's
// own Layer 5 counterpart: s applies to every battlefield permanent its own
// Affected$ valid-string matches, if s is a Mode$ Continuous line naming
// AddColor$ and/or SetColor$ in the one shape this slice can resolve -- a
// plain, " & "-separated list of literal color words (White/Blue/Black/
// Red/Green), plus the two fixed tokens "All" (WUBRG) and "Colorless" (no
// color at all, `SetColor$ Colorless`'s own real corpus shape).
//
// Skipped, the same reasons applyOneContinuousPT/applyOneContinuousType
// already give for their own params: Condition$/AffectedDefined$/
// AffectedZone$/CharacteristicDefining$. A "ChosenColor" token (7 of 61 real
// AddColor$/SetColor$ lines) skips the whole line -- a runtime value
// (Card.getChosenColors()) this port has no evaluator for, the identical
// "whole line, not partial" choice typeTokens already makes for ChosenType.
// 54 of 61 real lines carry none of it.
func applyOneContinuousColor(g *Game, host *Card, s *compile.Ability) {
	if !strings.EqualFold(s.Name, "Continuous") {
		return
	}
	for _, key := range [...]string{"Condition", "AffectedDefined", "AffectedZone", "CharacteristicDefining"} {
		if _, ok := s.Param(key); ok {
			return
		}
	}
	addColors, hasAdd := colorTokens(s, "AddColor")
	setColors, hasSet := colorTokens(s, "SetColor")
	if !hasAdd && !hasSet {
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
			c := g.Card(id)
			if hasSet {
				c.ColorMod.Add(ColorEffect{Timestamp: host.Timestamp, Colors: setColors, Overwrite: true})
			}
			if hasAdd {
				c.ColorMod.Add(ColorEffect{Timestamp: host.Timestamp, Colors: addColors})
			}
		}
	}
}

// colorTokens reads key (AddColor$ or SetColor$) as its own " & "-separated
// list of literal color words, unioned together via colorFromName (valid.go)
// -- the same per-word classification colorMatches uses, without its "non"
// prefix handling, which no real AddColor$/SetColor$ token carries. "All"
// resolves to every color (mana.AllColors) and "Colorless" to no color at
// all (mana.Colors(0), already the zero value) -- Java's own getColorsFromParam
// special-cases both the identical way. false, for the whole token list, the
// moment "ChosenColor" appears anywhere in it -- applyOneContinuousColor's
// own doc comment has the reason.
func colorTokens(s *compile.Ability, key string) (mana.Colors, bool) {
	v, ok := s.Param(key)
	if !ok {
		return 0, false
	}
	var out mana.Colors
	for _, word := range strings.Split(v, " & ") {
		switch word {
		case "ChosenColor":
			return 0, false
		case "All":
			out |= mana.AllColors
		case "Colorless":
			// No color at all -- contributes nothing to out, which is
			// exactly right for a lone "Colorless" token.
		default:
			c, ok := colorFromName(word)
			if !ok {
				return 0, false
			}
			out |= c
		}
	}
	return out, true
}

// applyContinuousKeyword recomputes every battlefield permanent's own Layer
// 6 KeywordMod effects from scratch, from every real Mode$ Continuous S:
// line currently in play -- applyContinuousPT's/applyContinuousType's own
// reasoning applies identically here.
func applyContinuousKeyword(g *Game) {
	for _, pid := range g.Players() {
		for _, id := range g.Zone(Battlefield, pid).Cards() {
			g.Card(id).KeywordMod.Clear()
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
					applyOneContinuousKeyword(g, h, s)
				}
			}
		}
	}
}

// applyOneContinuousKeyword is applyOneContinuousPT's/Type's/Color's own
// Layer 6 counterpart: s applies to every battlefield permanent its own
// Affected$ valid-string matches, if s is a Mode$ Continuous line naming
// AddKeyword$ in the one shape this slice can resolve -- a plain, " & "-
// separated list of literal keyword lines, already written exactly the way
// a real K: line would be ("Ward:2", "First Strike", "Protection:...") --
// keywordTokens (below) hands each one to KeywordEffect verbatim, and
// HasKeyword (card.go) reads them back with keyword.Parse the identical way
// it already reads a printed keyword.
//
// A whole line is skipped, not applied partially, the instant it carries:
//   - RemoveKeyword$/RemoveAllAbilities$ (5 of 1,561 real AddKeyword$
//     lines) -- this slice does not resolve either removal direction yet
//     (KeywordMod's own doc comment), and applying the add half of a "gains
//     X, loses Y" line without the remove half would leave the card with
//     both, an answer worse than the coverage gap of skipping the whole
//     line -- applyOneContinuousType's own "becomes a Turtle" paragraph
//     gives the identical reasoning.
//   - SharedKeywords$/FromDraftNotes$ -- a game-wide, remembered-list or
//     draft-note source for the keyword list rather than a fixed token list
//     (StaticAbilityContinuous.java's own alternate addKeywords-building
//     branches).
//   - a dynamic-value marker anywhere inside any one token (keywordTokens'
//     own doc comment has the full list, StaticAbilityContinuous.java's own
//     removeIf lambda) -- 42 of 1,857 real AddKeyword$ lines.
//
// 1,556 of 1,857 real AddKeyword$ lines carry none of the above and
// resolve.
func applyOneContinuousKeyword(g *Game, host *Card, s *compile.Ability) {
	if !strings.EqualFold(s.Name, "Continuous") {
		return
	}
	for _, key := range [...]string{
		"Condition", "AffectedDefined", "AffectedZone", "CharacteristicDefining",
		"RemoveKeyword", "RemoveAllAbilities", "SharedKeywords", "FromDraftNotes",
	} {
		if _, ok := s.Param(key); ok {
			return
		}
	}
	keywords, ok := keywordTokens(s, "AddKeyword")
	if !ok {
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
			g.Card(id).KeywordMod.Add(KeywordEffect{Timestamp: host.Timestamp, AddKeywords: keywords})
		}
	}
}

// keywordTokens reads key (AddKeyword$) as its own " & "-separated list of
// literal keyword lines, returned verbatim -- each token is exactly what a
// K: line would carry, HasKeyword's own job to parse further at query time,
// not this function's. false, for the whole line, the moment a
// dynamic-value marker (StaticAbilityContinuous.java's own removeIf lambda:
// ChosenColor, ChosenType, ChosenNumber, ChosenPlayer, ChosenName,
// ChosenEvenOdd, AllColors/allColors, CommanderColorID,
// ColorsYouCtrl/colorsYouCtrl, YourBasic) appears anywhere within any one
// token -- checked by substring, matching Java's own `input.contains(...)`,
// since a marker is often a qualifier embedded in a larger token
// ("Protection:Card.ChosenColor:chosenColor") rather than the whole token
// itself.
func keywordTokens(s *compile.Ability, key string) ([]string, bool) {
	v, ok := s.Param(key)
	if !ok {
		return nil, false
	}
	tokens := strings.Split(v, " & ")
	for _, tok := range tokens {
		for _, marker := range [...]string{
			"ChosenColor", "ChosenType", "ChosenNumber", "ChosenPlayer", "ChosenName",
			"ChosenEvenOdd", "chosenEvenOdd", "AllColors", "allColors", "CommanderColorID",
			"ColorsYouCtrl", "colorsYouCtrl", "YourBasic",
		} {
			if strings.Contains(tok, marker) {
				return nil, false
			}
		}
	}
	return tokens, true
}

// ptParam reads key as a plain base-10 integer (optionally negative) --
// AddPower$/AddToughness$/SetPower$/SetToughness$'s own corpus-frequent
// shape -- or, failing that, as the name of an SVar amounts defines (Java's
// own `ctb.getSVar(n)` lookup, xCount), resolved via resolveAmount
// (amount.go). Reports false for a missing key, or a value that is neither
// a plain integer nor a name amounts resolves (a genuinely dynamic value --
// AffectedX, ChosenNumber, xPaid, ... -- resolveAmount's own doc comment has
// the full account) -- the same "not resolvable, coverage gap rather than a
// wrong answer" contract compareMatches (valid.go) already documents.
func ptParam(g *Game, amounts map[string]expr.Amount, host *Card, s *compile.Ability, key string) (int, bool) {
	v, ok := s.Param(key)
	if !ok {
		return 0, false
	}
	if n, err := strconv.Atoi(v); err == nil {
		return n, true
	}
	amt, ok := amounts[strings.ToLower(v)]
	if !ok {
		return 0, false
	}
	return resolveAmount(g, amounts, host.Controller, host.ID, amt)
}

// applyContinuousRules recomputes every player's own Layer 8 RulesEffects
// from scratch, applyContinuousPT's own reasoning (above) applied to a
// player rather than a card: SetMaxHandSize$/RaiseMaxHandSize$/
// AdjustLandPlays$ Read Player.HandSizeLimit/LandPlayLimit (player.go).
func applyContinuousRules(g *Game) {
	for _, pid := range g.Players() {
		g.Player(pid).Rules.Clear()
	}
	for _, pid := range g.Players() {
		for _, host := range g.Zone(Battlefield, pid).Cards() {
			h := g.Card(host)
			if h.Def == nil {
				continue
			}
			for _, face := range h.Def.Faces {
				for _, s := range face.Statics {
					applyOneContinuousRules(g, h, face.Amounts, s)
				}
			}
		}
	}
}

// applyOneContinuousRules is Layer 8: s applies to every player its own
// Affected$ spec matches (matchesPlayerSpec, valid.go -- the identical
// dispatch every other player-shaped Affected/ValidPlayer/ValidActivatingPlayer
// check in this port already reuses, applied here against a static
// ability's Affected$ rather than a trigger's own player-shaped param), if
// s is a Mode$ Continuous line naming SetMaxHandSize$, RaiseMaxHandSize$
// and/or AdjustLandPlays$ in a shape rulesEffect (below) can resolve.
//
// Not resolved, each for a specific reason:
//   - Condition$/AffectedDefined$/AffectedZone$/CharacteristicDefining$ --
//     applyOneContinuousPT's own four skip reasons (a CharacteristicDefining
//     line makes no sense for a player-facing effect anyway; the one real
//     line carrying Condition$ alongside these three params, Delirium's own
//     "each opponent's maximum hand size is seven minus...", is skipped
//     here for that reason alone).
//   - MayLookAt$/MayPlay$ (88, 181 real lines corpus-wide) -- a cast-time
//     zone-eligibility permission CastSpell's own hand-only check
//     (castspell.go) has nowhere to consult yet.
//   - ControlOpponentsSearchingLibrary$/ControlVote$/AdditionalVote$/
//     AdditionalOptionalVote$/AdditionalVillainousChoice$/
//     DeclaresAttackers$/DeclaresBlockers$ (0-3 real lines each) --
//     multiplayer/vote mechanics this port has no concept of at all.
//   - IgnoreEffectCost$/AddHiddenKeyword$ (4, 19) -- each its own separate
//     mechanic (a cost-ignoring ability grant; a hidden functional keyword
//     whose own real values -- "must be blocked if able," "can't attack
//     alone," "doesn't untap," ... -- are each a distinct
//     block/attack/untap-step rule this port's own combat/turn model has no
//     hook for, none of them sharing enough machinery to be worth building
//     as one slice the way SetMaxHandSize/AdjustLandPlays do).
//   - A qualified Affected$ matchesPlayerSpec cannot resolve
//     (Player.NotedForGreenAnchor, Player.Chosen -- 1 real line each,
//     matchesPlayerSpec's own doc comment has the general reason).
//
// 75 of the corpus's 78 real SetMaxHandSize$/RaiseMaxHandSize$/
// AdjustLandPlays$ lines resolve here.
func applyOneContinuousRules(g *Game, host *Card, amounts map[string]expr.Amount, s *compile.Ability) {
	if !strings.EqualFold(s.Name, "Continuous") {
		return
	}
	for _, key := range [...]string{"Condition", "AffectedDefined", "AffectedZone", "CharacteristicDefining"} {
		if _, ok := s.Param(key); ok {
			return
		}
	}
	effect, ok := rulesEffect(g, host, amounts, s)
	if !ok {
		return
	}
	affected, ok := s.Param("Affected")
	if !ok {
		return
	}
	for _, pid := range g.Players() {
		matched, recognized := matchesPlayerSpec(g, pid, host.Controller, affected)
		if !recognized || !matched {
			continue
		}
		g.Player(pid).Rules.Add(effect)
	}
}

// rulesEffect reads s's own SetMaxHandSize$/RaiseMaxHandSize$/
// AdjustLandPlays$ params into one RulesEffect. "Unlimited" (Java's own
// literal sentinel for `p.setUnlimitedHandSize(true)`/
// `p.addMaxLandPlaysInfinite`) is checked before falling to ptParam (above)
// for the numeric case, since ptParam itself would just report it
// unresolvable (neither a plain integer nor a name amounts defines) --
// correctly, on its own terms, but the caller here needs to tell "no
// maximum" apart from "genuinely could not resolve this." ok is false the
// moment ANY dimension s names cannot be resolved, not just the ones that
// can -- applyOneContinuousType's own "skip the whole line rather than
// apply it partially" contract, ported here even though no real corpus
// line currently names more than one of the three at once.
func rulesEffect(g *Game, host *Card, amounts map[string]expr.Amount, s *compile.Ability) (RulesEffect, bool) {
	e := RulesEffect{Timestamp: host.Timestamp}
	any := false

	if v, ok := s.Param("SetMaxHandSize"); ok {
		if strings.EqualFold(v, "Unlimited") {
			e.HasSetHandSize, e.SetHandSizeUnlimited, any = true, true, true
		} else if n, ok := ptParam(g, amounts, host, s, "SetMaxHandSize"); ok {
			e.HasSetHandSize, e.SetHandSize, any = true, n, true
		} else {
			return RulesEffect{}, false
		}
	}
	if _, ok := s.Param("RaiseMaxHandSize"); ok {
		n, ok := ptParam(g, amounts, host, s, "RaiseMaxHandSize")
		if !ok {
			return RulesEffect{}, false
		}
		e.HasRaiseHandSize, e.RaiseHandSize, any = true, n, true
	}
	if v, ok := s.Param("AdjustLandPlays"); ok {
		if strings.EqualFold(v, "Unlimited") {
			e.HasAdjustLandPlays, e.AdjustLandPlaysUnlimited, any = true, true, true
		} else if n, ok := ptParam(g, amounts, host, s, "AdjustLandPlays"); ok {
			e.HasAdjustLandPlays, e.AdjustLandPlays, any = true, n, true
		} else {
			return RulesEffect{}, false
		}
	}
	return e, any
}
