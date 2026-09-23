// Destroy: CR 701.7, M6's own next script-driven effect after the twelve
// castspell.go's own NewRegistry doc comment already lists -- 986 of the
// corpus's real (AB|DB)$ Destroy lines (excluding DestroyAll, a separate
// ApiType/effect class this port does not build), 782 of them naming
// ValidTgts$ and 205 naming Defined$ instead -- SpellAbilityEffect.
// getTargetCards(sa)'s own either/or contract, targetedOrDefinedCards
// (defined.go).
//
// Ported from
// forge-game/src/main/java/forge/game/ability/effects/DestroyEffect.java's
// resolve/internalDestroy. Not ported: GameAction.destroy's own
// ReplacementType.Destroy check (CR 701.16's own regeneration shield) --
// replacement.go builds no Destroy-type replacement at all, so running the
// check would always answer NotReplaced; skipping it changes nothing a real
// game could observe, the identical reasoning NoRegen$ below gets.
// TriggerType.Destroyed -- a rare corpus trigger mode ("whenever a permanent
// is destroyed," distinct from Mode$ Dies, which checkDiesTriggers already
// fires below for every real battlefield departure regardless of cause) this
// port's own trigger table has no entry for yet, the identical "an unbuilt
// Mode$ simply never fires" reasoning every other still-missing mode already
// has (game-state.md's own "Not ported yet").
package engine

import (
	"fmt"

	"github.com/jczastkiewicz/crucible/internal/cardtype"
)

// destroyUnresolvedParams names DestroyEffect's own params this port does
// not evaluate. Every one fails the whole line loudly rather than destroying
// the wrong permanent or silently dropping half of what it asked for
// (PORT-8/GO-7): Condition$/ConditionDefined$ (0/16) -- condition.go's own
// subAbilityConditionMet would otherwise silently no-op a card naming
// either, dealDamageEffect's own identical reasoning; PlayerTurn$ (7) --
// unclear semantics on a Destroy line, not worth guessing at, pumpEffect's
// own identical reasoning; SorcerySpeed$ (9) -- a cost-restriction flag with
// no cost-payment site to attach to, sacrificeEffect's own identical
// reasoning; Ultimate$ (11) -- a planeswalker-ultimate-specific flag, its
// own further mechanic; ModeCost$ (7) -- Charm's own per-mode cost linkage,
// unrelated; RememberLKI$ (10) -- needs an LKI snapshot of the destroyed
// permanent taken before the move, this port's own Game.LKI is a whole-batch
// zone-change record (game-state.md), not wired to a single script-driven
// effect's own Remembered list yet.
//
// NoRegen$ (62) is not here: this port has no regeneration shield to skip
// (the file doc comment above has the reason), so the param changes nothing
// this port's own resolve body does either way -- a real, narrow
// simplification, not a gap.
//
// SubAbility$ chains through resolveSubAbility (subability.go,
// Registry.Resolve, effect.go) once this effect's own body finishes, the
// identical shape every other M6 effect already has. UnlessCost$/
// UnlessPayer$/UnlessResolveSubs$/UnlessSwitched$ no longer block either:
// resolveUnlessCost (effect.go) gates the whole ability before Registry.
// Resolve ever reaches it.
var destroyUnresolvedParams = [...]string{
	"Condition", "ConditionDefined", "PlayerTurn", "SorcerySpeed", "Ultimate",
	"ModeCost", "RememberLKI",
}

// destroyEffect resolves Mode$/DB$/AB$ Destroy. ConditionPresent$/
// ConditionCompare$/ConditionCheckSVar$/ConditionSVarCompare$ are resolved
// through subAbilityConditionMet (condition.go), the identical way every
// other M6 effect's own does.
type destroyEffect struct{}

func (destroyEffect) Resolve(g *Game, a *Ability, controller PlayerController) error {
	for _, key := range destroyUnresolvedParams {
		if _, ok := a.Params.Param(key); ok {
			return fmt.Errorf("engine: Destroy: %s$ not resolvable yet", key)
		}
	}
	source := g.Card(a.Source)
	if !subAbilityConditionMet(g, source, a.Amounts, a.Params) {
		return nil
	}
	cards, err := targetedOrDefinedCards(source, a.Params, a.Targets)
	if err != nil {
		return fmt.Errorf("engine: Destroy: %w", err)
	}
	_, remember := a.Params.Param("RememberDestroyed")
	var destroyed []CardID
	for _, id := range cards {
		c := g.Card(id)
		if c.Zone != Battlefield || !canBeDestroyed(c) {
			continue
		}
		if remember {
			g.Card(a.Source).Memory.Remember(CardEntity(id))
		}
		g.Move(id, Graveyard, c.Owner)
		g.checkDiesTriggers(controller, id)
		destroyed = append(destroyed, id)
	}
	g.checkChangesZoneAllTriggers(controller, destroyed, Battlefield, Graveyard)
	return nil
}

// canBeDestroyed is Card.canBeDestroyed's own formula, minus the isInPlay/
// isPhasedOut halves the caller already checks (phasing does not exist in
// this port -- game-state.md's own "Not ported yet"): indestructible blocks
// destruction unless c is a creature at 0 or less toughness, CR 704.5f's own
// carve-out (destroyDamagedCreatures's own identical Indestructible/
// Toughness reasoning, action.go, applied here to an explicit Destroy rather
// than lethal damage).
func canBeDestroyed(c *Card) bool {
	if !c.HasKeyword("Indestructible") {
		return true
	}
	if !c.Type().Has(cardtype.Creature) {
		return false
	}
	t, ok := c.Toughness()
	return ok && t <= 0
}
