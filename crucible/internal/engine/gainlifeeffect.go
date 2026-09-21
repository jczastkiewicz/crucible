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
// field; CR 119's own "life gain replacement" family (Event$ GainLife, 21
// real replacement lines) has real content now too, both checked per player
// before Life is touched at all: gainLifePrevented (replacement.go) resolves
// sulfuric_vortex.txt's own bare Prevent$ True, the only one of the 21
// naming Prevent$ at all; gainLifeReplaced (replacement.go) resolves 4 of
// the other 20's own ReplaceWith$ lines -- GainDouble/RLoseLife/Draw among
// them -- once ReplaceCount$LifeGained, "the amount of life that would have
// been gained" as a runtime value, reads back through a narrow sibling of
// resolveAmount rather than resolveAmount itself (replacement.go's own doc
// comment has the full reason and the 16 that stay unresolved). LifeChanged,
// below, is the identical event dealPlayerDamage already emits for a life
// LOSS, reused here for a gain.
//
// Not ported (every one fails loudly rather than granting the wrong amount
// to the wrong player, PORT-8/GO-7): Planeswalker$/UnlessPayer$/UnlessCost$/
// ValidTgts$ (each its own further mechanic, and this port's own targeting
// gap for the non-Defined$ shape); Condition$ itself and ConditionDefined$/
// ConditionZone$/ConditionOptionalPaid$ (SpellAbilityCondition's own
// separate flag switch and shapes subAbilityConditionMet does not cover,
// the identical DealDamage-shaped gap).
//
// ConditionPresent$/ConditionCompare$/ConditionCheckSVar$/ConditionSVarCompare$
// are resolved through subAbilityConditionMet (condition.go) the identical
// way DealDamage's own do. SubAbility$ chains through resolveSubAbility
// (subability.go, effect.go's own Registry.Resolve) once this effect's own
// body finishes -- CR's own "then" (Sphinx Sovereign, Rogue Refiner, ...)
// -- and runs whether or not subAbilityConditionMet above let this effect's
// own body run at all, resolveApiAbility's own unconditional
// resolveSubAbilities pairing (subability.go's own doc comment). 18 of the
// corpus's own 253 real SVar-defined GainLife lines naming SubAbility$
// chain to an already-built leaf ability (no further SubAbility$ of its
// own) and resolve end to end; a chain more than one deep, or one whose
// target is not built yet, is not counted here -- each of those effects'
// own count already tracks that half of the question.
type gainLifeEffect struct{}

var gainLifeUnresolvedParams = [...]string{
	"Planeswalker", "UnlessPayer", "UnlessCost", "ValidTgts",
	"Condition", "ConditionDefined", "ConditionZone", "ConditionOptionalPaid",
}

func (gainLifeEffect) Resolve(g *Game, a *Ability, controller PlayerController) error {
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
	players, err := definedPlayers(g, a.Controller, defined, a.Targets)
	if err != nil {
		return fmt.Errorf("engine: GainLife: %w", err)
	}
	for _, pid := range players {
		if g.gainLifePrevented(pid) {
			continue
		}
		if g.gainLifeReplaced(controller, pid, amount) {
			continue
		}
		g.Player(pid).Life += amount
		g.sink.Emit(Event{Kind: LifeChanged, Source: a.Source, Target: PlayerEntity(pid), Amount: int32(amount)})
		g.checkLifeGainedTriggers(controller, pid)
	}
	return nil
}
