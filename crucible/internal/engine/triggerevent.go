// A trigger's conditions read against the event that fired it: Java's
// Trigger.checkWith (Trigger.java:428-446). An "intervening if" condition can
// name the event's triggering objects -- "if no opponent has more life than
// that player" is CheckSVar$ TriggeredDefendingPlayer$LifeTotal (Veteran
// Soldier), "if it's attacking the player with the most life" is
// CheckDefinedPlayer$ TriggeredDefender.withMostLife (Scourge of the Throne).
// Java builds the triggering objects once per event (TriggerWaiting
// .getTriggeringObjects) and lets the check read them; without an event in
// hand it leaves those two conditions out (Trigger.paramsWithoutEvent, :413).
//
// This port's trigger scans test a trigger's conditions (triggerEffectAPI)
// before they build its triggering objects. So the scan leaves the two
// conditions out the way Java does without an event, and they are read
// once the match carries its objects: eventConditionsHold, at the top of
// pushTriggeredAbilities and resolveStaticTriggers, the two places every scan
// hands its matches to. A delayed trigger's scan has its objects before its
// check, so it is checked with them directly (delayedTriggerAPI).

package engine

import (
	"strings"

	"github.com/jczastkiewicz/crucible/internal/carddb/compile"
	"github.com/jczastkiewicz/crucible/internal/expr"
)

// eventConditionKeys are the condition params Trigger.paramsWithoutEvent
// leaves out when they name a triggering object (Trigger.java:413).
var eventConditionKeys = [...]string{"CheckSVar", "CheckDefinedPlayer"}

// readsTriggeringObjects is paramsWithoutEvent's test: the param's value, or
// the SVar it names, mentions "Trigger".
func readsTriggeringObjects(amounts map[string]expr.Amount, t *compile.Ability, key string) bool {
	v, ok := t.Param(key)
	if !ok {
		return false
	}
	if strings.Contains(v, "Trigger") {
		return true
	}
	amt, ok := amounts[strings.ToLower(v)]
	return ok && (strings.Contains(amt.Head, "Trigger") || strings.Contains(amt.Body, "Trigger") || strings.Contains(amt.Context, "Trigger"))
}

// hasEventConditions reports whether t carries a condition only an event can
// answer.
func hasEventConditions(amounts map[string]expr.Amount, t *compile.Ability) bool {
	for _, key := range eventConditionKeys {
		if readsTriggeringObjects(amounts, t, key) {
			return true
		}
	}
	return false
}

// withoutEventConditions is Trigger.paramsWithoutEvent: t with the
// conditions that name a triggering object left out, t itself when it has
// none.
func withoutEventConditions(amounts map[string]expr.Amount, t *compile.Ability) *compile.Ability {
	if !hasEventConditions(amounts, t) {
		return t
	}
	cut := *t
	cut.Params = nil
	for _, p := range t.Params {
		drop := false
		for _, key := range eventConditionKeys {
			if strings.EqualFold(p.Key, key) && readsTriggeringObjects(amounts, t, key) {
				drop = true
			}
		}
		if !drop {
			cut.Params = append(cut.Params, p)
		}
	}
	return &cut
}

// withTriggerEvent runs check with the trigger of source reading objects as
// its event (Trigger.checkWith), restoring whatever check was in progress.
func (g *Game) withTriggerEvent(source CardID, controller PlayerID, objects triggeredObjects, check func() bool) bool {
	saved := g.triggerEvent
	g.triggerEvent = &triggerEventCheck{source: source, controller: controller, objects: objects}
	defer func() { g.triggerEvent = saved }()
	return check()
}

// eventConditionsHold drops the matches whose event conditions do not hold
// against their own triggering objects: the half of canRunTrigger the scan
// left out (withoutEventConditions). A condition this port cannot read
// records a pending error and drops the match (GO-7).
func (g *Game) eventConditionsHold(matches []Ability) []Ability {
	kept := matches[:0:0]
	for _, m := range matches {
		if m.trigger == nil || m.eventChecked || !hasEventConditions(m.Amounts, m.trigger) {
			kept = append(kept, m)
			continue
		}
		host := g.Card(m.Source)
		holds := g.withTriggerEvent(m.Source, host.Controller(), m.triggered, func() bool {
			return checkDefinedPlayerMatches(g, host, m.Amounts, m.trigger) &&
				checkSVarMatches(g, host, m.Amounts, m.trigger, "CheckSVar", "SVarCompare", "CheckSecondSVar")
		})
		if holds {
			m.eventChecked = true
			kept = append(kept, m)
		}
	}
	return kept
}

// triggerRefs is what a Triggered* reference read for source sees: the event
// a trigger of source is being checked against (Trigger.getTriggeringObject,
// Trigger.java:448-454), else source's own resolving ability. ok is false
// when neither is in hand.
func (g *Game) triggerRefs(source CardID) (refs abilityRefs, controller PlayerID, ok bool) {
	if e := g.triggerEvent; e != nil && source != NoCard && e.source == source {
		return abilityRefs{triggered: e.objects, game: g, activator: e.controller}, e.controller, true
	}
	if a := g.resolving; a != nil && source != NoCard && a.Source == source {
		return a.refs(), a.Controller, true
	}
	return abilityRefs{}, NoPlayer, false
}

// interveningIfHolds is WrappedAbility.resolve's recheck (CR 603.4,
// WrappedAbility.java:402-412): a triggered ability off the stack checks its
// trigger's conditions again, against the triggering objects it fired with,
// before the "you may" is asked or any cost is paid. Trigger.requirementsCheck
// is the common conditions (triggerCommonRequirementsMet) and ResolvedLimit$;
// the zones, phases and ActivationLimit$ are not checked again. Mode$ Always
// (its ResolvingCheck$ instead, recheckFails) and NoResolvingCheck$ True
// lines skip it. A failed recheck leaves the stack with no effect and no
// resolution counted, yet AbilityResolves still fires: MagicStack.java:712-718
// runs it after AbilityUtils.resolve whatever the wrapper did (quirks.md,
// PORT-7). A static trigger resolves at the instant its conditions were
// read, so it has nothing to recheck.
func (g *Game) interveningIfHolds(a *Ability) bool {
	if !a.isTrigger || !a.stackRoot || a.Params == nil {
		return true
	}
	t := a.trigger
	if a.firedDelayed != nil {
		t = a.firedDelayed
	}
	if t == nil || strings.EqualFold(t.Name, "Always") {
		return true
	}
	if _, ok := t.Param("NoResolvingCheck"); ok {
		return true
	}
	host := g.Card(a.Source)
	reader := host.Controller()
	if a.firedDelayed != nil {
		reader = a.Controller
	}
	return g.withTriggerEvent(a.Source, reader, a.triggered, func() bool {
		return triggerCommonRequirementsMet(g, host, a.Amounts, t) && triggerResolvedLimitMet(host, t, a.Params)
	})
}
