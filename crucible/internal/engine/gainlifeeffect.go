// GainLife: CR 119.1, M6's third script-driven effect, and the corpus's
// single largest resolvable slice past DealDamage -- a plain-or-named-SVar
// LifeAmount$ granted to a Defined$ player, no target -- 862 of the corpus's
// 1,700 real (AB|DB)$ GainLife lines that also name Defined$
// You/Player.Opponent and carry no other unresolved param -- 5 of them
// naming Planeswalker$ too, Ajani Goldmane's own real "[+1]: You gain 2
// life" among them, no longer blocked (below): CR 606.3's own loyalty
// ability restriction is a cost-side gate (ActivateAbility/
// ActivateManaAbility, activateability.go), never a restriction on how the
// effect it pays for actually resolves.
//
// Ported from
// forge-game/src/main/java/forge/game/ability/effects/LifeGainEffect.java's
// resolve.

package engine

//enginelint:allow id card game player ability event trigger defined amount condition control

import "fmt"

// gainLifeEffect resolves Mode$/DB$/AB$ GainLife. Player.Life is a plain
// field; CR 119's own "life gain replacement" family (Event$ GainLife, 21
// real replacement lines) has real content now too, both checked per player
// before Life is touched at all: gainLifePrevented (replacement.go) resolves
// sulfuric_vortex.txt's own bare Prevent$ True, the only one of the 21
// naming Prevent$ at all; gainLifeReplaced (replacement.go) carries CR 616's
// own two outcomes for the other 20, a full substitution (19 of them --
// Draw/LoseLife/ReplaceEffect target shapes, gainLifeReplaced's own doc
// comment has the full corpus accounting and the 1 that stays unresolved) or
// a resized gain, the same "reduce or replace the amount, keep going through
// the normal path" contract damagePrevented's/damageReplaced's own split
// already has for a different Event$ -- gain, below, is the number
// gainLifeReplaced itself says should actually be granted, already run
// through both outcomes. LifeChanged, below, is the identical event
// dealPlayerDamage already emits for a life LOSS, reused here for a gain.
//
// Not ported (every one fails loudly rather than granting the wrong amount
// to the wrong player, PORT-8/GO-7): ValidTgts$ (this port's own targeting
// gap for the non-Defined$ shape); Condition$ itself and ConditionDefined$/ConditionZone$/
// ConditionOptionalPaid$ (SpellAbilityCondition's own separate flag switch
// and shapes subAbilityConditionMet does not cover, the identical
// DealDamage-shaped gap).
//
// UnlessCost$/UnlessPayer$ no longer block either: resolveUnlessCost
// (effect.go) gates the whole ability before Registry.Resolve ever reaches
// it. 0 of the corpus's own 3 real GainLife lines naming UnlessCost$ clear
// that gate's own pure-mana-cost/resolvable-payer filter at all (each
// names a Sac<.../Reveal<... cost part or a controller-derived UnlessPayer$
// this port cannot resolve).
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
	"ValidTgts",
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
	players, err := definedPlayers(g, a.Controller, a.Source, defined, a.refs())
	if err != nil {
		return fmt.Errorf("engine: GainLife: %w", err)
	}
	for _, pid := range players {
		if g.gainLifePrevented(pid) {
			continue
		}
		gain := g.gainLifeReplaced(controller, pid, amount)
		if gain <= 0 {
			continue
		}
		firstGain := g.Player(pid).LifeGainedTimesThisTurn == 0
		g.Player(pid).LifeGainedTimesThisTurn++
		g.Player(pid).Life += gain
		g.sink.Emit(Event{Kind: LifeChanged, Source: a.Source, Target: PlayerEntity(pid), Amount: int32(gain)})
		g.checkLifeGainedTriggers(controller, pid, firstGain)
	}
	return nil
}
