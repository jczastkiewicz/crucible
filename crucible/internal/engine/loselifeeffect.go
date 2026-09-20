// LoseLife: CR 119.3, GainLife's own mirror image -- a plain-or-named-SVar
// LifeAmount$ taken from a Defined$ player, no target -- 226 of the corpus's
// 445 real (AB|DB)$ LoseLife lines that also name Defined$
// You/Opponent/Player.Opponent and carry no other unresolved param.
//
// Ported from
// forge-game/src/main/java/forge/game/ability/effects/LifeLoseEffect.java's
// resolve.

package engine

import "fmt"

// loseLifeEffect resolves Mode$/DB$/AB$ LoseLife. Player.Life is a plain
// field with no combat-style prevention machinery built for it yet
// (LifeChanged, below, is the identical event dealPlayerDamage already
// emits for a life loss, reused here): CR 119's own "life reduced"
// replacement family (ReplacementType.LifeReduced) is not ported, the
// identical "real gap, not a wrong answer" gainLifeEffect's own unbuilt
// "life gain replacement" remainder already is. Java's own
// Player.loseLife also fires TriggerType.LifeLost/LifeLostAll -- 0 real
// T:Mode$ LifeLost/LifeLostAll lines corpus-wide, so unlike
// checkLifeGainedTriggers (gainLifeEffect's own real caller), nothing here
// checks a trigger at all.
//
// Not ported (every one fails loudly rather than draining the wrong amount
// from the wrong player, PORT-8/GO-7): SubAbility$ -- no ability-chaining
// mechanism exists yet; Planeswalker$/UnlessPayer$/UnlessCost$/
// UnlessSwitched$/ValidTgts$ (each its own further mechanic, and this
// port's own targeting gap for the non-Defined$ shape); Ultimate$/
// IsPresent$/PresentCompare$/NumCards$/ModeCost$ (unclear semantics or
// each its own further mechanic, not worth guessing at from a handful of
// real lines); Condition$ itself and ConditionDefined$/ConditionZone$
// (SpellAbilityCondition's own separate flag switch and shapes
// subAbilityConditionMet does not cover, the identical GainLife-shaped
// gap).
//
// ConditionPresent$/ConditionCompare$/ConditionCheckSVar$/ConditionSVarCompare$
// are resolved through subAbilityConditionMet (condition.go) the identical
// way GainLife's own do.
type loseLifeEffect struct{}

var loseLifeUnresolvedParams = [...]string{
	"SubAbility", "Planeswalker", "UnlessPayer", "UnlessCost", "UnlessSwitched", "ValidTgts",
	"Ultimate", "IsPresent", "PresentCompare", "NumCards", "ModeCost",
	"Condition", "ConditionDefined", "ConditionZone",
}

func (loseLifeEffect) Resolve(g *Game, a *Ability) error {
	for _, key := range loseLifeUnresolvedParams {
		if _, ok := a.Params.Param(key); ok {
			return fmt.Errorf("engine: LoseLife: %s$ not resolvable yet", key)
		}
	}
	source := g.Card(a.Source)
	if !subAbilityConditionMet(g, source, a.Amounts, a.Params) {
		return nil
	}
	lifeAmount, ok := a.Params.Param("LifeAmount")
	if !ok {
		return fmt.Errorf("engine: LoseLife: LifeAmount$ missing")
	}
	amount, ok := resolveNamedAmount(g, a.Amounts, source, lifeAmount)
	if !ok {
		return fmt.Errorf("engine: LoseLife: LifeAmount$ %q is not resolvable", lifeAmount)
	}
	defined, _ := a.Params.Param("Defined")
	players, err := definedPlayers(g, a.Controller, defined)
	if err != nil {
		return fmt.Errorf("engine: LoseLife: %w", err)
	}
	for _, pid := range players {
		g.Player(pid).Life -= amount
		g.sink.Emit(Event{Kind: LifeChanged, Source: a.Source, Target: PlayerEntity(pid), Amount: -int32(amount)})
	}
	return nil
}
