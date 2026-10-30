package engine

//enginelint:allow ability additional amount card condition control defined effecthelpers game id trigger

import (
	"fmt"
	"strings"

	"github.com/jczastkiewicz/crucible/internal/carddb/compile"
)

// immediateTriggerEffect is ImmediateTriggerEffect.java: a reflexive
// triggered ability (CR 603.12) -- TriggerAmount$ (default 1) copies of
// Execute$, each put on the stack as a triggered ability of this ability's
// controller, remembering RememberObjects$ (one object per copy with
// RememberEach$). Java registers it as a delayed trigger that fires at the
// next trigger check; this port pushes it at once, which lands it on the
// stack above everything the resolving ability left there, the same
// resolution order. OptionalDecider$ You makes it a "may". The copies carry
// the spawning ability's X.
// RememberSVarAmount$ remembers an Integer (NumberEntity), which
// Count$TriggerRememberAmount sums; Static$ and the triggering-object copies
// fail closed.
type immediateTriggerEffect struct{}

func (immediateTriggerEffect) Resolve(g *Game, a *Ability, controller PlayerController) error {
	if err := rejectUnreadCondition(a, a.Params.Name); err != nil {
		return err
	}
	if err := rejectParams(a, "ImmediateTrigger", "Static", "AfterReplacement"); err != nil {
		return err
	}
	source := g.Card(a.Source)
	if !subAbilityConditionMet(g, source, a.Amounts, a.Params) {
		return nil
	}
	amount, err := optionalAmount(g, a, "ImmediateTrigger", "TriggerAmount", 1)
	if err != nil || amount <= 0 {
		return err
	}
	execs := additionalAbilities(a.Params, "Execute")
	if len(execs) == 0 {
		return fmt.Errorf("engine: ImmediateTrigger: no Execute$")
	}
	if execs[0].Ability.Name == compile.BackRefName {
		resolved, ok := g.resolveBackRef(source, execs[0].SVar)
		if !ok {
			return fmt.Errorf("engine: ImmediateTrigger: Execute$ %s is not on the chain's card", execs[0].SVar)
		}
		execs[0].Ability = resolved
	}
	api, ok := APIByName(execs[0].Ability.Name)
	if !ok {
		return fmt.Errorf("engine: ImmediateTrigger: Execute$ %s: unrecognized API %q", execs[0].SVar, execs[0].Ability.Name)
	}
	optional, ok := triggerIsOptional(a.Params)
	if !ok {
		return fmt.Errorf("engine: ImmediateTrigger: OptionalDecider$ not resolvable yet")
	}
	var remember []EntityID
	if raw, ok := a.Params.Param("RememberObjects"); ok {
		for _, def := range strings.Split(raw, " & ") {
			es, err := definedEntities(g, a.Controller, source, def, a.refs())
			if err != nil {
				return fmt.Errorf("engine: ImmediateTrigger: %w", err)
			}
			remember = append(remember, es...)
		}
	}
	// RememberSVarAmount$ (ImmediateTriggerEffect.java:83-87): the named SVar's
	// amount, remembered as an Integer after the objects.
	hasAmount := false
	rememberedAmount := 0
	if name, ok := a.Params.Param("RememberSVarAmount"); ok {
		n, known := resolveNamedAmount(g, a.Amounts, source, name)
		if !known {
			return fmt.Errorf("engine: ImmediateTrigger: RememberSVarAmount$ %q is not resolvable", name)
		}
		hasAmount, rememberedAmount = true, n
	}
	each := hasParam(a, "RememberEach")
	if each && len(remember) < amount {
		return fmt.Errorf("engine: ImmediateTrigger: RememberEach$ has %d objects for %d triggers", len(remember), amount)
	}
	// Spawner>Sacrificed$CardPower (Heart-Piercer Manticore): the copies read
	// the cost this ability paid, as it was at payment (LKI ids).
	spawnerPaid := a.paid
	spawnerTriggered := a.triggered
	var abilities []Ability
	for i := 0; i < amount; i++ {
		r := remember
		if each {
			r = remember[i : i+1]
		}
		if hasAmount {
			r = append(append([]EntityID(nil), r...), NumberEntity(rememberedAmount))
		}
		// Count$xPaid in the reflexive trigger reads the X of the ability that
		// spawned it, 0 when that one announced none (AbilityUtils.java:
		// 1649-1656): Isareth's "pay {X}. When you do, ... mana value X".
		abilities = append(abilities, Ability{
			API: api, Source: a.Source, Controller: a.Controller, Params: execs[0].Ability,
			Amounts: a.Amounts, Optional: optional, TriggerRemembered: r,
			xManaCostPaid: a.xManaCostPaid, hasXManaCostPaid: true, spawnerPaid: &spawnerPaid, spawnerTriggered: &spawnerTriggered,
		})
	}
	g.pushTriggeredAbilities(controller, abilities)
	return nil
}

// resolveBackRef is the compiled ability SVar named name on the card the
// running chain belongs to: the effect card's origin (the card whose trait
// started the chain, through regrants), else the host itself.
func (g *Game) resolveBackRef(host *Card, name string) (*compile.Ability, bool) {
	for _, c := range []*Card{host, g.Card(host.effectOrigin)} {
		if c == nil || c.Def == nil {
			continue
		}
		for fi := range c.Def.Faces {
			f := &c.Def.Faces[fi]
			var roots []*compile.Ability
			roots = append(roots, f.Abilities...)
			roots = append(roots, f.Triggers...)
			roots = append(roots, f.Statics...)
			roots = append(roots, f.Replacements...)
			for _, r := range roots {
				if found := findSVarAbility(r, name); found != nil {
					return found, true
				}
			}
		}
	}
	return nil, false
}

// findSVarAbility walks a's finite compiled tree for the ability defined in
// SVar name; a back-reference stub ends a branch.
func findSVarAbility(a *compile.Ability, name string) *compile.Ability {
	if a.SVar == name && a.Name != compile.BackRefName {
		return a
	}
	for _, s := range a.Subs {
		if s.Ability != nil && s.Ability.Name != compile.BackRefName {
			if found := findSVarAbility(s.Ability, name); found != nil {
				return found
			}
		}
	}
	return nil
}
