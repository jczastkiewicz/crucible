// Activating an ability: CR 602, trimmed to the corpus's own two dominant
// cost shapes -- pure mana, and pure mana plus a single Tap-self token --
// the only ones this port has a payment primitive for. Java's own entry
// point (Player.playSpellAbility, by way of PlayerControllerHuman/AI's own
// input loop) is a real priority-window action; this port has no priority
// window at all yet (game-state.md's own "Not ported yet" -- "ResolveStack
// plays out only the degenerate case, nobody able to respond"), so timing
// collapses to the identical sorcery-speed shape CastSpell's own CR 601.3a
// simplification already uses: active player, a main phase, an empty
// stack. A future instant-speed activation needs the real priority window
// built first, not a special case here.

package engine

import (
	"strings"

	"github.com/jczastkiewicz/crucible/internal/carddb/compile"
	"github.com/jczastkiewicz/crucible/internal/cost"
	"github.com/jczastkiewicz/crucible/internal/mana"
)

// ActivateAbility is CR 602.2: pay index's own Cost$, then the ability
// becomes an object on the stack (CR 602.2g/405.2) -- resolving is
// ResolveStack's own separate step, identical to a cast spell. index
// selects among card's own compiled `A:` lines (Card.Def.Faces[0].
// Abilities, compile.go) by position, the same "caller already knows the
// card's own script" contract a fixture author already has for everything
// else this port drives by index rather than by name.
//
// Reports whether the ability was activated. false covers every
// legal-but-declined case CastSpell's own bool return already covers (wrong
// timing, not pid's own permanent, not on the battlefield, the cost could
// not be paid) plus three more specific to this shape: index does not name
// an Activated-record line at all (a card's own `A:SP$` line, an Instant's
// or Adventure's own spell half, shares the same Abilities slice -- compile.
// go's own doc comment on why both `A:` kinds land there together), the
// line's own API is "Mana" (CR 605.3a's own no-stack immediate resolution,
// a wholly different mechanism this port only has for a basic land's own
// intrinsic ability, TapLandForMana, manaability.go -- extending it to an
// arbitrary permanent's own printed mana ability is not this shape), or the
// line's own Cost$ is not IsPureManaOrTap (internal/cost) -- a named Part
// past the lone "T" that token itself always also parses as (Sac<.../
// Discard<.../PayLife<.../...), or an Untap/Mandatory/XMin token, each its
// own further payment primitive this port does not have, PORT-8/GO-7's
// "skip the whole line" applied to the cost itself rather than to the
// ability's own other params.
//
// A Tap-self cost checks CR 602.5b/302.6 first, with no side effect yet:
// already tapped, or summoning-sick without haste, both decline outright --
// DeclareCombatAttackers' own identical SummonSick/Haste check
// (attack.go), reused rather than re-derived. The mana half is paid through
// PayManaCost exactly as CastSpell's own is; only once that succeeds does
// the tap itself actually happen (Card.Tapped set, checkTapsTriggers fired)
// -- CR 602.2g's own "costs are paid together" is approximated here as
// "check every cost for feasibility first, then commit each one," so a
// failed mana payment never leaves the permanent tapped for nothing.
//
// A successful activation pushes through pushTriggeredAbilities
// (trigger.go) with card's own controller as the sole entry -- resolving
// ValidTgts$ (targeting.go) and firing CR 115's own "becomes the target"
// check the identical way a triggered ability's own push already does,
// APNAP ordering a harmless no-op over the one player activating. No
// "activates an ability" trigger mode exists to check afterward
// (CR 603's own remaining gap, game-state.md); this port has none built to
// fire.
func (g *Game) ActivateAbility(pid PlayerID, card CardID, index int, controller PlayerController) bool {
	if pid != g.activePlayer {
		return false
	}
	if g.activePhase != Main1 && g.activePhase != Main2 {
		return false
	}
	if len(g.stack) != 0 {
		return false
	}
	c := g.Card(card)
	if c.Controller() != pid || c.Zone != Battlefield {
		return false
	}
	abilities := c.Def.Faces[0].Abilities
	if index < 0 || index >= len(abilities) {
		return false
	}
	ability := abilities[index]
	if ability.Record != compile.Activated || ability.Name == "Mana" {
		return false
	}
	costText, ok := ability.Param("Cost")
	if !ok {
		return false
	}
	parsed := cost.Parse(costText)
	if !parsed.IsPureManaOrTap() {
		return false
	}
	if parsed.Tap && (c.Tapped || (c.SummonSick && !c.HasKeyword("Haste"))) {
		return false
	}
	manaCost, err := mana.Parse(strings.Join(parsed.Mana, " "))
	if err != nil {
		return false
	}
	apiType, ok := APIByName(ability.Name)
	if !ok {
		return false
	}
	if !g.PayManaCost(pid, manaCost, controller) {
		return false
	}
	if parsed.Tap {
		c.Tapped = true
		g.checkTapsTriggers(controller, card, pid, false)
	}
	g.pushTriggeredAbilities(controller, []Ability{{
		API: apiType, Source: card, Controller: pid,
		Params: ability, Amounts: c.Def.Faces[0].Amounts,
	}})
	return true
}
