// Activating an ability: CR 602, trimmed to the corpus's own dominant cost
// shapes -- mana, an optional Tap-self token, an optional self-sacrifice
// token (Sac<1/CARDNAME>), and an optional "discard N cards of your choice"
// (Discard<N/Card>), in any combination (cost.Cost.ActivationShape,
// internal/cost) -- the only primitives this port has payment machinery
// for. Java's own entry
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
// line's own Cost$ has no ActivationShape (internal/cost) -- a chosen or
// SVar-sized Sac<...>, a Discard<...> past the literal "N/Card" shape, a
// PayLife<.../PayEnergy<.../... part ActivationShape does not carry at all,
// or an Untap/Mandatory/XMin token, each its own further payment primitive
// this port does not have, PORT-8/GO-7's "skip the whole line" applied to
// the cost itself rather than to the ability's own other params, or a
// Discard component the activating player's own hand cannot actually pay
// (fewer cards in hand than DiscardN).
//
// Every feasibility check runs before anything is committed: a Tap-self
// cost checks CR 602.5b/302.6 first (already tapped, or summoning-sick
// without haste, DeclareCombatAttackers' own identical SummonSick/Haste
// check, attack.go, reused rather than re-derived), and a Discard component
// checks the hand actually holds DiscardN cards. The mana half is paid
// through PayManaCost exactly as CastSpell's own is; only once that
// succeeds does the tap itself actually happen (Card.Tapped set,
// checkTapsTriggers fired), then a self-sac cost actually sacrifices the
// card (sacrificeCards, sacrificeeffect.go, reused wholesale -- CR 701.20's
// own "dies" trigger, RememberSacrificed$, and the batched
// Mode$ ChangesZoneAll firing all come free, exactly as they already do for
// Sacrifice's own "Self" branch), then a Discard component asks
// ChooseCardsToDiscard for exactly DiscardN cards and discards them
// (discardCards, discardeffect.go, reused wholesale the identical way) --
// CR 602.2g's own "costs are paid together" is approximated here as "check
// every cost for feasibility first, then commit each one, mana first, tap
// second, sacrifice third, discard last," so a failed mana payment never
// leaves the permanent tapped, sacrificed, or the player short a card for
// nothing, and a Tap-self cost never taps a permanent that has already left
// the battlefield. CR 601.2h's own "costs may be paid in any order" makes
// this ordering a free choice, not an approximation of a specific one
// Java's own CostPayment (a part-by-part, player-cancellable payment loop
// this port does not build) would make instead.
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
	shape, ok := parsed.ActivationShape()
	if !ok {
		return false
	}
	if shape.Tap && (c.Tapped || (c.SummonSick && !c.HasKeyword("Haste"))) {
		return false
	}
	hand := g.Zone(Hand, pid).Cards()
	if shape.DiscardN > len(hand) {
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
	activated := Ability{
		API: apiType, Source: card, Controller: pid,
		Params: ability, Amounts: c.Def.Faces[0].Amounts,
	}
	if !g.PayManaCost(pid, manaCost, controller) {
		return false
	}
	if shape.Tap {
		c.Tapped = true
		g.checkTapsTriggers(controller, card, pid, false)
	}
	if shape.SelfSac {
		sacrificeCards(g, controller, &activated, []CardID{card})
	}
	if shape.DiscardN > 0 {
		chosen := controller.ChooseCardsToDiscard(g, pid, hand, shape.DiscardN)
		discardCards(g, controller, chosen, pid)
	}
	g.pushTriggeredAbilities(controller, []Ability{activated})
	return true
}
