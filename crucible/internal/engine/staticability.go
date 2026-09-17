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

	"github.com/jczastkiewicz/crucible/internal/keyword"
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
// Landwalk is not in this table: its own CantBlockBy carries no ValidBlocker
// at all, only ValidDefender$ Player.controls<Type>, and <Type> is the
// keyword's OWN argument (K:Landwalk:Island's own "Island") -- a different
// value per card, not a name shared by every card carrying the keyword the
// way Fear's/Flying's/Horsemanship's/Intimidate's fixed ValidBlocker strings
// are. landwalkType (below) reads it directly off the keyword line instead
// (enchantSpec's own precedent, action.go).
//
// Not every keyword CardFactoryUtil expands this way is here -- each
// remaining omission is a specific missing dependency, not an oversight:
//
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
					vd, hasVD := s.Param("ValidDefender")
					if applyCantBlockBy(g, h, va, vb, hasVB, vd, hasVD, attacker, blocker) {
						return true
					}
				}
			}
			for _, kb := range cantBlockByKeywords {
				if h.HasKeyword(kb.keyword) && applyCantBlockBy(g, h, "Creature.Self", kb.validBlocker, true, "", false, attacker, blocker) {
					return true
				}
			}
			if typ, ok := landwalkType(h); ok &&
				applyCantBlockBy(g, h, "Creature.Self", "", false, "Player.controls"+typ, true, attacker, blocker) {
				return true
			}
		}
	}
	return false
}

// landwalkType reports h's own Landwalk keyword argument (K:Landwalk:Island
// -> "Island", K:Landwalk:Forest.Snow:snow Forest -> "Forest.Snow" -- the
// keyword's own first Args() element, exactly Landwalk.java's own
// getValidType/KeywordWithType.type), and whether h carries the keyword at
// all. Read directly off the keyword line rather than through
// cantBlockByKeywords' fixed-string table, since this value is the one part
// of Landwalk's own CantBlockBy synthesis
// (CardFactoryUtil.java's `Landwalk landwalk` branch) that differs per card.
func landwalkType(h *Card) (string, bool) {
	if h.Def == nil {
		return "", false
	}
	for _, line := range h.Def.Faces[0].Keywords {
		k := keyword.Parse(line)
		if k.Name != "Landwalk" {
			continue
		}
		args := k.Args()
		if len(args) == 0 || args[0] == "" {
			continue
		}
		return args[0], true
	}
	return "", false
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
// card; and the Landwalk ignore-check (StaticAbilityIgnoreLandwalk.java) --
// zero real corpus S:Mode$ IgnoreLandwalk lines exist, so nothing here can
// ever need to consult it.
func applyCantBlockBy(g *Game, host *Card, validAttacker, validBlocker string, hasValidBlocker bool,
	validDefender string, hasValidDefender bool, attacker, blocker CardID) bool {
	if !Matches(g, g.Card(attacker), valid.Parse(validAttacker), host.Controller, host.ID) {
		return false
	}
	if hasValidBlocker && !Matches(g, g.Card(blocker), valid.Parse(validBlocker), host.Controller, host.ID) {
		return false
	}
	if hasValidDefender && !matchesValidDefender(g, g.Card(blocker).Controller, validDefender, host) {
		return false
	}
	return true
}

// matchesValidDefender is ValidDefender's own check
// (StaticAbilityCantAttackBlock.applyCantBlockByAbility:
// `stAb.matchesValidParam("ValidDefender", blocker.getController())`) -- a
// Player, not a Card, so Matches (valid.go) cannot evaluate it.
// matchesPlayerBase's own three bare values (valid.go) cover 6 of the 8 real
// literal ValidDefender$ lines, checked here against defender vs
// host.Controller. A "Player.controls<Type>" value -- Landwalk's own entire
// restriction, landwalkType's own doc comment has the reason it is built
// per card rather than looked up -- asks whether defender controls at least
// one battlefield permanent valid.Parse(type) matches (PlayerProperty.java's
// own "controls" branch, `property.substring(8)`, no comparator suffix:
// every real corpus use of this shape is the bare "at least one" default).
// Any other value (Player.Condition, Card.Self -- 2 of the 8 real literal
// lines) never matches, the same skip-rather-than-fire contract every other
// unresolved param in this port gets.
func matchesValidDefender(g *Game, defender PlayerID, spec string, host *Card) bool {
	if matched, ok := matchesPlayerBase(defender, host.Controller, spec); ok {
		return matched
	}
	if typ, ok := strings.CutPrefix(spec, "Player.controls"); ok {
		return controllerControlsType(g, defender, typ, host)
	}
	return false
}

// controllerControlsType reports whether pid controls at least one
// battlefield permanent valid.Parse(typeSpec) matches, source/sourceController
// the ability's own host -- Matches's own "source is the card the spec is
// written on" contract (valid.go), the same pairing every other
// staticability.go check already passes.
func controllerControlsType(g *Game, pid PlayerID, typeSpec string, host *Card) bool {
	spec := valid.Parse(typeSpec)
	for _, id := range g.Zone(Battlefield, pid).Cards() {
		if Matches(g, g.Card(id), spec, host.Controller, host.ID) {
			return true
		}
	}
	return false
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
