// LoseLife: CR 119.3, GainLife's own mirror image -- a plain-or-named-SVar
// LifeAmount$ taken from a Defined$ or targeted player -- 300 of the
// corpus's 445 real (AB|DB)$ LoseLife lines that name Defined$
// You/Opponent/Player.Opponent or a resolvable ValidTgts$ and carry no
// other unresolved param.
//
// Ported from
// forge-game/src/main/java/forge/game/ability/effects/LifeLoseEffect.java's
// resolve, whose own `getTargetPlayers(sa)` (SpellAbilityEffect.java) is
// the reason ValidTgts$ and Defined$ are mutually exclusive here rather
// than both consulted: getTargetPlayers reads sa.getTargets() directly when
// the ability uses targeting at all, never falling through to
// AbilityUtils.getDefinedPlayers's own Defined$ switch in that case -- and
// 0 real LoseLife lines combine ValidTgts$ with a Defined$ of their own,
// confirming the corpus never relies on the fallback coexisting.
// resolveTargets (targeting.go) already resolved ValidTgts$'s own
// candidates and stored the answer on a.Targets by the time this runs
// (pushTriggeredAbilities, trigger.go), so this reads that directly rather
// than routing it through definedPlayers's own "Targeted"/"TargetedPlayer"
// cases (defined.go) -- those exist for a literal `Defined$ Targeted` a
// different real corpus line writes explicitly, not for this shape.

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
// from the wrong player, PORT-8/GO-7): Planeswalker$/Ultimate$/IsPresent$/
// PresentCompare$/NumCards$/ModeCost$ (unclear semantics or each its own
// further mechanic, not worth guessing at from a handful of real lines);
// Condition$ itself and ConditionDefined$/ConditionZone$
// (SpellAbilityCondition's own separate flag switch and shapes
// subAbilityConditionMet does not cover, the identical GainLife-shaped
// gap). ValidTgts$ itself no longer blocks: resolveTargets (targeting.go)
// resolves the two real player-shaped bases (Opponent/Player, 74 of 76
// real ValidTgts$ lines) -- the other 2, qualified
// Player.wasDealtDamageThisTurnBySource/Player.LostLifeThisTurn, still do
// not resolve, matchesPlayerProperty's own unrecognized-property contract
// (valid.go) leaving them with zero legal targets rather than a wrong one.
//
// ConditionPresent$/ConditionCompare$/ConditionCheckSVar$/ConditionSVarCompare$
// are resolved through subAbilityConditionMet (condition.go) the identical
// way GainLife's own do. SubAbility$ chains through resolveSubAbility
// (subability.go, effect.go's own Registry.Resolve) once this effect's own
// body finishes, whether or not subAbilityConditionMet above let this
// effect's own body run at all -- Sphinx Sovereign's own real shape ("you
// gain 3 life if untapped. Otherwise, each opponent loses 3 life" is one
// DB$ LoseLife with a SubAbility$ DB$ GainLife, the negated condition split
// across the two) is exactly why resolveApiAbility's own
// resolveSubAbilities call in Java is unconditional. 144 of the corpus's
// own 382 real SVar-defined LoseLife lines naming SubAbility$ chain to an
// already-built leaf ability and resolve end to end; a chain more than one
// deep, or one whose target is not built yet, is not counted here.
//
// UnlessCost$/UnlessPayer$/UnlessSwitched$ no longer block either:
// resolveUnlessCost (effect.go) gates the whole ability before
// Registry.Resolve ever reaches it. 0 of the corpus's own 42 real LoseLife
// lines naming UnlessCost$ resolve, though -- delaying_shield.txt's own
// real pure-mana "{1}{W}" is the only one clearing resolveUnlessCost's own
// pure-mana-cost/resolvable-payer filter, and it is reached only through
// DB$ Repeat's own RepeatSubAbility$ (not the plain SubAbility$ chaining
// this port's own resolveSubAbility reads), a general repeat-N-times
// mechanic this port does not build; every other real line names a
// Sac<.../Discard<.../PayLife<.../... cost part or a controller-derived
// UnlessPayer$ (TriggeredPlayer, ParentTarget, ...) this port cannot
// resolve.
type loseLifeEffect struct{}

var loseLifeUnresolvedParams = [...]string{
	"Planeswalker",
	"Ultimate", "IsPresent", "PresentCompare", "NumCards", "ModeCost",
	"Condition", "ConditionDefined", "ConditionZone",
}

func (loseLifeEffect) Resolve(g *Game, a *Ability, _ PlayerController) error {
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

	var players []PlayerID
	if _, hasValidTgts := a.Params.Param("ValidTgts"); hasValidTgts {
		for _, e := range a.Targets {
			if pid, ok := e.AsPlayer(); ok {
				players = append(players, pid)
			}
		}
	} else {
		defined, _ := a.Params.Param("Defined")
		var err error
		players, err = definedPlayers(g, a.Controller, defined, a.Targets)
		if err != nil {
			return fmt.Errorf("engine: LoseLife: %w", err)
		}
	}
	for _, pid := range players {
		g.Player(pid).Life -= amount
		g.sink.Emit(Event{Kind: LifeChanged, Source: a.Source, Target: PlayerEntity(pid), Amount: -int32(amount)})
	}
	return nil
}
