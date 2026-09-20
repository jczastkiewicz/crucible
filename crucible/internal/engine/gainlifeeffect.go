// GainLife: CR 119.1, M6's third script-driven effect, and the corpus's
// single largest resolvable slice past DealDamage -- a plain-or-named-SVar
// LifeAmount$ granted to a Defined$ player, no target -- 857 of the corpus's
// 1,700 real (AB|DB)$ GainLife lines that also name Defined$
// You/Player.Opponent and carry no other unresolved param.
//
// Ported from
// forge-game/src/main/java/forge/game/ability/effects/LifeGainEffect.java's
// resolve.

package engine

import "fmt"

// gainLifeEffect resolves Mode$/DB$/AB$ GainLife. Player.Life is a plain
// field with no combat-style prevention machinery built for it yet
// (LifeChanged, below, is the identical event dealPlayerDamage already
// emits for a life LOSS, reused here for a gain): CR 119's own "life gain
// replacement" family (Event$ GainLife, 22 real replacement lines, 2
// Prevent$ True and 20 ReplaceWith$-driven) is not ported, the identical
// "real gap, not a wrong answer" this port's own untapped/unresolved
// replacement remainders already are (game-state.md's "Not ported yet").
//
// Not ported (every one fails loudly rather than granting the wrong amount
// to the wrong player, PORT-8/GO-7): SubAbility$ -- no ability-chaining
// mechanism exists yet; Planeswalker$/UnlessPayer$/UnlessCost$/ValidTgts$
// (each its own further mechanic, and this port's own targeting gap for the
// non-Defined$ shape); Condition$ itself and ConditionDefined$/
// ConditionZone$/ConditionOptionalPaid$ (SpellAbilityCondition's own
// separate flag switch and shapes subAbilityConditionMet does not cover,
// the identical DealDamage-shaped gap).
//
// ConditionPresent$/ConditionCompare$/ConditionCheckSVar$/ConditionSVarCompare$
// are resolved through subAbilityConditionMet (condition.go) the identical
// way DealDamage's own do.
type gainLifeEffect struct{}

var gainLifeUnresolvedParams = [...]string{
	"SubAbility", "Planeswalker", "UnlessPayer", "UnlessCost", "ValidTgts",
	"Condition", "ConditionDefined", "ConditionZone", "ConditionOptionalPaid",
}

func (gainLifeEffect) Resolve(g *Game, a *Ability, _ PlayerController) error {
	for _, key := range gainLifeUnresolvedParams {
		if _, ok := a.Params.Param(key); ok {
			return fmt.Errorf("engine: GainLife: %s$ not resolvable yet", key)
		}
	}
	source := g.Card(a.Source)
	if !subAbilityConditionMet(g, source, a.Amounts, a.Params) {
		return nil
	}
	lifeAmount, ok := a.Params.Param("LifeAmount")
	if !ok {
		return fmt.Errorf("engine: GainLife: LifeAmount$ missing")
	}
	amount, ok := resolveNamedAmount(g, a.Amounts, source, lifeAmount)
	if !ok {
		return fmt.Errorf("engine: GainLife: LifeAmount$ %q is not resolvable", lifeAmount)
	}
	defined, _ := a.Params.Param("Defined")
	players, err := definedPlayers(g, a.Controller, defined)
	if err != nil {
		return fmt.Errorf("engine: GainLife: %w", err)
	}
	for _, pid := range players {
		g.Player(pid).Life += amount
		g.sink.Emit(Event{Kind: LifeChanged, Source: a.Source, Target: PlayerEntity(pid), Amount: int32(amount)})
		g.checkLifeGainedTriggers(pid)
	}
	return nil
}
