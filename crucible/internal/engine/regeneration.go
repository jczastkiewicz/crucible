package engine

import (
	"strings"

	"github.com/jczastkiewicz/crucible/internal/carddb/compile"
	"github.com/jczastkiewicz/crucible/internal/expr"
	"github.com/jczastkiewicz/crucible/internal/valid"
)

// regenerate is CR 701.16's own regeneration action, applied whenever a
// permanent that would be destroyed has a way to regenerate instead
// (Card.canRegenerate's own two real sources): a regeneration shield --
// Card.RegenShields, granted by AB$/DB$ Regenerate (regenerateeffect.go) --
// or a permanent's own always-on "if this would be destroyed, regenerate
// it" replacement (ApiType.Regeneration -- RegenerationEffect.java --
// reached only through R:Event$ Destroy | Regeneration$ True | ReplaceWith$
// <SVar>, e.g. mossbridge_troll.txt:5-6). It reports whether the
// destruction was replaced.
//
// The static replacement is checked first: it costs nothing and Java's own
// GameAction.destroy offers every applicable ReplacementEffect through the
// identical CR 616 "which one applies" choice a shield's own replacement
// instance is generated into -- checking it first here simply means a
// permanent carrying both this static text AND an active shield spends
// neither at once and keeps the shield for a later destruction, the more
// useful of the two orders a corpus with no card combining both today
// cannot tell apart empirically either way.
func (g *Game) regenerate(controller PlayerController, id CardID) bool {
	c := g.Card(id)
	if destroyReplacedByRegeneration(g, c) {
		g.regenerateBody(controller, id)
		return true
	}
	if c.RegenShields <= 0 {
		return false
	}
	c.RegenShields--
	g.regenerateBody(controller, id)
	return true
}

// regenerateBody is CR 701.16's own three-part action -- RegenerationEffect
// .resolve's own healDamage/tap/removeFromCombat trio -- shared by both of
// regenerate's own sources: a spent shield and the static replacement
// (destroyReplacedByRegeneration) below.
func (g *Game) regenerateBody(controller PlayerController, id CardID) {
	c := g.Card(id)
	c.Damage.Clear()
	if !c.Tapped {
		c.Tapped = true
		g.checkTapsTriggers(controller, id, c.Controller(), false)
	}
	g.removeFromCombat(id)
}

// destroyReplacedByRegeneration reports whether host carries a resolvable
// "if this would be destroyed, regenerate it" replacement (ApiType
// .Regeneration's own one real corpus shape, mossbridge_troll.txt:5-6,
// knight_of_the_holy_nimbus.txt:6-7, clergy_of_the_holy_nimbus.txt:6-7 --
// 3 of 3 real DB$ Regeneration lines, every one naming ValidCard$ Card.Self,
// so host is always both the replacement's own carrier and the card the
// replacement is about).
//
// Hand-run directly against host's own compiled Replacements, the identical
// "recognize the one shape, run it by hand" precedent drawReplaced's/
// gainLifeReplaced's own applyDrawReplacement/applyGainLifeReplacement
// already have for a different Event$ (replacement.go): the call sites that
// decide a destruction (destroyEffect, destroyAllEffect,
// destroyDamagedCreatures) run outside any Registry.Resolve call, so
// Game.registry (game.go) cannot be relied on to be set yet -- a fresh
// game's very first state-based-action check, before any ability has ever
// resolved, is exactly the case that would panic on a nil Registry.
//
// ApiType.Regeneration is not registered in NewRegistry for the identical
// reason Draw's/PutCounter's own ReplaceWith$ shapes never gained a second
// registration of their own: every real corpus line reaching it does so
// through this one hand-run path, never through an ordinary AB$/DB$ chain
// a card script could name on its own (0 real AB$/SP$ Regeneration lines
// exist). scripts/unported-apis.sh still counts these 3 lines as
// unregistered on that basis; effects-batch-b.md notes the discrepancy.
func destroyReplacedByRegeneration(g *Game, host *Card) bool {
	if host.Def == nil {
		return false
	}
	for _, face := range host.Def.Faces {
		for _, r := range face.Replacements {
			if !regenerationReplacementMatches(g, r, host, face.Amounts) {
				continue
			}
			for _, sub := range r.Subs {
				if strings.EqualFold(sub.Key, "ReplaceWith") && regenerationSubAbilityRecognized(sub.Ability) {
					return true
				}
			}
		}
	}
	return false
}

// regenerationReplacementMatches is destroyReplacedByRegeneration's own
// shared half: Event$ Destroy, Regeneration$ True, host in one of r's own
// ActiveZones$ (hostInActiveZones, replacement.go -- always Battlefield in
// every real line and at every real call site, since regenerate is only
// ever asked about a permanent already there), ValidCard$ matched against
// host itself the identical way every other replacement match in this
// package already is, and replacementRequirementsCheck (replacement.go).
// Any param besides the ones the corpus's own 3 real lines carry (Event,
// ActiveZones, ValidCard, Regeneration, ReplaceWith, Description) skips the
// whole line rather than guessing (GO-7) -- there are none today, so this
// allow-list is exhaustive against the real corpus, not aspirational.
func regenerationReplacementMatches(g *Game, r *compile.Ability, host *Card, amounts map[string]expr.Amount) bool {
	if !strings.EqualFold(r.Name, "Destroy") {
		return false
	}
	regen, ok := r.Param("Regeneration")
	if !ok || !strings.EqualFold(regen, "True") {
		return false
	}
	for _, p := range r.Params {
		switch strings.ToLower(p.Key) {
		case "event", "regeneration", "description", "validcard", "activezones", "replacewith", "secondary":
		default:
			return false
		}
	}
	if !hostInActiveZones(host, r, host.Zone) {
		return false
	}
	validCard, ok := r.Param("ValidCard")
	if !ok {
		return false
	}
	if !Matches(g, host, valid.Parse(validCard), host.Controller(), host.ID) {
		return false
	}
	return replacementRequirementsCheck(g, host, amounts, r)
}

// regenerationSubAbilityRecognized is tapAbilityResolvesTap's own
// "recognize this ReplaceWith$ shape" role (replacement.go) applied to
// ApiType.Regeneration: a bare `DB$ Regeneration | Defined$ ReplacedCard`
// (3 of 3 real lines) or `Defined$ Self` (0 real lines, but the identical
// substitution AbilityUtils.getDefinedCards would make for it, since every
// real line's own ValidCard$ Card.Self already means the two resolve to the
// same card). ReplacedCard is Java's own "the object the replacement is
// actually about," identical here to Self because ValidCard$ Card.Self is
// the only real shape -- a future replacement whose ValidCard$ names a
// DIFFERENT card would need a general "replacing object" context this port
// does not carry (drawReplaced's/gainLifeReplaced's own doc comments note
// the same gap for their own Event$s), so this substitution is narrow to
// this one shape, not a general Defined$ ReplacedCard reader. Any other
// param, or a SubAbility$ of its own, is refused rather than run partially
// (PORT-8/GO-7).
func regenerationSubAbilityRecognized(a *compile.Ability) bool {
	if !strings.EqualFold(a.Name, "Regeneration") {
		return false
	}
	defined, ok := a.Param("Defined")
	if !ok {
		return false
	}
	for _, p := range a.Params {
		switch strings.ToLower(p.Key) {
		case "db", "defined":
		default:
			return false
		}
	}
	return strings.EqualFold(defined, "ReplacedCard") || strings.EqualFold(defined, "Self")
}
