// Two static-ability modes with no layer-folding of their own: CR 509.1b's
// CantBlockBy (flying/reach, Fear, Horsemanship, and every real corpus S:
// line written in that shape) and the legend rule's own IgnoreLegendRule
// corner case. Neither needs CR 613's layer system -- each is a plain
// ValidCard/ValidAttacker/ValidBlocker match, same shape valid.go already
// evaluates for everything else -- which is what makes both buildable ahead
// of Mode$ Continuous itself (game-state.md's "Continuous effects" section
// has the reasoning in full).
//
// Ported from
// forge-game/src/main/java/forge/game/staticability/StaticAbilityCantAttackBlock.java's
// cantBlockBy/applyCantBlockByAbility, and
// forge-game/src/main/java/forge/game/staticability/StaticAbilityIgnoreLegendRule.java.

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
	{"Intimidate", "Creature.nonArtifact+!SharesColorWith"},
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

// ignoreLegendRule reports whether id is exempt from the legend rule (CR
// 704.5j) by some Mode$ IgnoreLegendRule static ability in play. Ported from
// StaticAbilityIgnoreLegendRule.ignoreLegendRule/applyIgnoreLegendRuleAbility:
// every battlefield permanent is walked as a possible host (Java's own
// STATIC_ABILITIES_SOURCE_ZONES, trimmed to Battlefield the same way
// cantBlockBy's own doc comment justifies), and a ValidCard-less line (1 of
// the 11 real corpus lines, an unconditional "the legend rule doesn't
// apply") matches every card, exactly Java's own
// `stAb.matchesValidParam("ValidCard", card)` contract for an absent param.
//
// Not ported: a line carrying IsPresent$/PresentCompare$ (2 of the 11 --
// "if you control exactly two permanents named X") -- StaticAbility.java's
// own checkConditions evaluates those generically for every static-ability
// mode, a mechanism this port has not built for any mode yet (game-state.md's
// "Not ported yet"). Skipped rather than guessed at: an ability this port
// cannot evaluate the condition for is treated as not currently active, the
// same safe default an unresolvable Toughness leaves a creature alive under
// (destroyLethalToughness's own doc comment, action.go) -- wrong only in the
// rare case the condition holds, never in the far more common case it does
// not.
func ignoreLegendRule(g *Game, id CardID) bool {
	for _, pid := range g.Players() {
		for _, host := range g.Zone(Battlefield, pid).Cards() {
			h := g.Card(host)
			if h.Def == nil {
				continue
			}
			for _, face := range h.Def.Faces {
				for _, s := range face.Statics {
					if !strings.EqualFold(s.Name, "IgnoreLegendRule") {
						continue
					}
					if _, ok := s.Param("IsPresent"); ok {
						continue
					}
					validCard, ok := s.Param("ValidCard")
					if !ok {
						return true
					}
					if Matches(g, g.Card(id), valid.Parse(validCard), h.Controller, h.ID) {
						return true
					}
				}
			}
		}
	}
	return false
}
