// Block legality beyond "untapped": CR 509.1b's CantBlockBy static ability
// (flying/reach, Fear, Horsemanship, and every real corpus S: line written
// in that shape), the general mechanism block.go's own doc comment named
// as a gap.
//
// Ported from
// forge-game/src/main/java/forge/game/staticability/StaticAbilityCantAttackBlock.java's
// cantBlockBy/applyCantBlockByAbility.

package engine

import (
	"strings"

	"github.com/jczastkiewicz/crucible/internal/valid"
)

// cantBlockByKeywords is every keyword CardFactoryUtil.java turns into a
// Mode$ CantBlockBy static ability at CardState-build time
// (CardFactoryUtil.java:3906-3935), not a combat-code keyword check --
// Flying's own block restriction is this static ability, generated once for
// every card that carries the keyword, the same way Fear's and
// Horsemanship's are (block.go's own doc comment already established this
// for Flying specifically). ValidAttacker is always "Creature.Self" for a
// keyword-synthesized one, since Java attaches the synthesized
// StaticAbility to the very card carrying the keyword.
//
// Not every keyword CardFactoryUtil expands this way is here -- each
// omission is a specific missing dependency, not an oversight:
//
//   - Intimidate's own ValidBlocker ("nonArtifact+!SharesColorWith") needs a
//     SharesColorWith property valid.go does not evaluate. Leaving it
//     unimplemented is not just "absent": propertyMatches' own generic
//     non<Type> fallthrough would read "SharesColorWith" as a nonexistent
//     type (false), and the leading "!" negates that to true -- an actively
//     wrong "always matches," not a missing one, so Intimidate is left out
//     rather than shipped wrong.
//   - Landwalk's CantBlockBy carries no ValidBlocker at all, only
//     ValidDefender$ Player.controls<Type> -- Matches (valid.go) evaluates a
//     *Card, not a *Player, so nothing here can check it yet.
//   - Protection needs Protection.java's own valid-string builder
//     (Protection.getProtectionValid), not a fixed string.
//   - Skulk's ValidBlocker$ Creature.powerGTX needs an SVar-driven X;
//     compareMatches (valid.go) already documents a non-numeric Compare
//     operand as unresolvable, so a Skulk entry here would just never match,
//     silently wrong for its one real job.
//
// Menace is not here either, but for a different reason: Forge itself does
// not run Menace through the static-ability engine at all --
// StaticAbilityCantAttackBlock.getMinMaxBlocker hardcodes
// `attacker.hasKeyword(Keyword.MENACE)` directly, a minimum-blocker-COUNT
// rule CantBlockBy's per-blocker-identity check cannot express. Porting it
// needs a different hook (validating the size of a Block group per
// attacker, not a single pair) -- a real gap, not a cut corner
// (docs/crucible/porting/port-log/game-state.md's "Block legality"
// section).
var cantBlockByKeywords = []struct {
	keyword      string
	validBlocker string
}{
	{"Flying", "Creature.withoutFlying+withoutReach"},
	{"Fear", "Creature.nonArtifact+nonBlack"},
	{"Horsemanship", "Creature.withoutHorsemanship"},
}

// cantBlockBy reports whether attacker cannot legally be blocked by blocker,
// per every Mode$ CantBlockBy static ability currently in play.
//
// Every battlefield permanent is walked as a possible host, not just
// attacker itself -- Java's own cantBlockBy walks every card in
// ZoneType.STATIC_ABILITIES_SOURCE_ZONES (Battlefield, Graveyard, Exile,
// Command, Stack), because a real corpus S:CantBlockBy line is as often
// written on an Aura or Equipment (ValidAttacker$
// Creature.EnchantedBy/EquippedBy, granting its host "can't be blocked") as
// on the attacker's own card. Only Battlefield is walked here: no real
// corpus CantBlockBy line needs a source in the other four zones
// (game-state.md's "Not ported yet" has the count this is based on).
func cantBlockBy(g *Game, attacker, blocker CardID) bool {
	for _, pid := range g.Players() {
		for _, host := range g.Zone(Battlefield, pid).Cards() {
			h := g.Card(host)
			if h.Def == nil {
				continue
			}
			for _, face := range h.Def.Faces {
				for _, s := range face.Statics {
					if !strings.EqualFold(s.Name, "CantBlockBy") {
						continue
					}
					va, ok := s.Param("ValidAttacker")
					if !ok {
						continue
					}
					vb, hasVB := s.Param("ValidBlocker")
					if applyCantBlockBy(g, h, va, vb, hasVB, attacker, blocker) {
						return true
					}
				}
			}
			for _, kb := range cantBlockByKeywords {
				if h.HasKeyword(kb.keyword) && applyCantBlockBy(g, h, "Creature.Self", kb.validBlocker, true, attacker, blocker) {
					return true
				}
			}
		}
	}
	return false
}

// applyCantBlockBy is applyCantBlockByAbility's ValidAttacker/ValidBlocker
// half, the only two params any real corpus S:CantBlockBy line or
// keyword-synthesized one this port builds actually needs (ValidAttacker in
// every real line; ValidBlocker absent only on a handful of unconditional
// "can't be blocked" lines, applied here exactly as Java does: skipped
// rather than treated as "matches nothing"). host is the card the ability
// lives on -- Creature.Self in validAttacker/validBlocker resolves against
// it, not against attacker or blocker, matching Matches's own "source is
// the card the spec is written on" contract (valid.go). validBlocker's
// comma-separated alternatives need no manual splitting: valid.Parse already
// treats a comma as OR between Spec.Alternatives, the same as any other
// multi-alternative valid string this port already passes through whole
// (trigger.go's ValidCard$ Cleric.Other,Card.Self is the same shape).
//
// Not ported from applyCantBlockByAbility: the "Dragon Hunter" reach
// exception (a ValidBlocker alternative containing "withoutReach" is undone
// if a separate CanBlockIfReach static grants that specific blocker
// effective reach against this specific attacker) -- one real corpus card
// needs it; ValidAttackerRelative/ValidBlockerRelative -- one real corpus
// card; ValidDefender -- zero real corpus S:CantBlockBy lines use it; and
// the Landwalk ignore-check (StaticAbilityIgnoreLandwalk.java) -- Landwalk
// itself is not in cantBlockByKeywords, so this never reaches a case that
// would need it (game-state.md's "Not ported yet" has the corpus counts).
func applyCantBlockBy(g *Game, host *Card, validAttacker, validBlocker string, hasValidBlocker bool, attacker, blocker CardID) bool {
	if !Matches(g, g.Card(attacker), valid.Parse(validAttacker), host.Controller, host.ID) {
		return false
	}
	if hasValidBlocker && !Matches(g, g.Card(blocker), valid.Parse(validBlocker), host.Controller, host.ID) {
		return false
	}
	return true
}
