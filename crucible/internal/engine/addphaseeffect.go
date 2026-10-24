package engine

//enginelint:allow ability card condition control effecthelpers game phase delayedtrigger additional

import (
	"fmt"
	"strings"
)

// addPhaseEffect is AddPhaseEffect.java (CR 500.8): NumPhases$ (default 1)
// times, the ExtraPhase$ -- Beginning (untap, upkeep, draw), Combat (the
// six combat steps) or one named phase -- then FollowedBy$ is inserted
// after AfterPhase$ (default the current phase), the most recent insertion
// running first (PhaseHandler.addExtraPhase). ExtraPhaseDelayedTrigger$
// (AddPhaseEffect.java:70-76) hangs a delayed trigger on the first phase of
// each insertion, registered when that phase is entered.
// BeforeFirstPostCombatMainEnd$ (World at War) adds nothing after the first
// post-combat main phase.
type addPhaseEffect struct{}

func (addPhaseEffect) Resolve(g *Game, a *Ability, _ PlayerController) error {
	if err := rejectUnreadCondition(a, a.Params.Name); err != nil {
		return err
	}
	source := g.Card(a.Source)
	if !subAbilityConditionMet(g, source, a.Amounts, a.Params) {
		return nil
	}
	if hasParam(a, "BeforeFirstPostCombatMainEnd") && !g.beforeFirstPostCombatMainEnd() {
		// AddPhaseEffect.java:34-38: presence only; no extra phase once more
		// than the first post-combat main has begun.
		return nil
	}
	after := g.activePhase
	if raw, ok := a.Params.Param("AfterPhase"); ok {
		p, ok := phaseNameFold(strings.TrimSpace(raw))
		if !ok {
			return fmt.Errorf("engine: AddPhase: AfterPhase$ %q not resolvable", raw)
		}
		after = p
	}
	extra, ok := a.Params.Param("ExtraPhase")
	if !ok {
		return fmt.Errorf("engine: AddPhase: no ExtraPhase$")
	}
	var list []PhaseType
	switch strings.TrimSpace(extra) {
	case "Beginning":
		list = []PhaseType{Untap, Upkeep, Draw}
	case "Combat":
		list = []PhaseType{CombatBegin, DeclareAttackers, DeclareBlockers, FirstStrikeDamage, CombatDamage, CombatEnd}
	default:
		p, ok := phaseNameFold(strings.TrimSpace(extra))
		if !ok {
			return fmt.Errorf("engine: AddPhase: ExtraPhase$ %q not resolvable", extra)
		}
		list = []PhaseType{p}
	}
	if raw, ok := a.Params.Param("FollowedBy"); ok {
		p, ok := phaseNameFold(strings.TrimSpace(raw))
		if !ok {
			return fmt.Errorf("engine: AddPhase: FollowedBy$ %q not resolvable", raw)
		}
		list = append(list, p)
	}
	num, err := optionalAmount(g, a, "AddPhase", "NumPhases", 1)
	if err != nil {
		return err
	}
	var trigger *delayedTrigger
	if subs := additionalAbilities(a.Params, "ExtraPhaseDelayedTrigger"); len(subs) > 0 {
		if mode, _ := subs[0].Ability.Param("Mode"); !strings.EqualFold(mode, "Phase") {
			return fmt.Errorf("engine: AddPhase: ExtraPhaseDelayedTrigger$ Mode$ %q not resolvable yet", mode)
		}
		trigger = &delayedTrigger{
			Trigger: subs[0].Ability, Host: a.Source, Controller: a.Controller, Amounts: a.Amounts,
			ThisTurn: true, HostTransforms: source.Transforms,
		}
	}
	natural := PhaseType((int(after) + 1) % numPhaseTypes)
	for n := num; n > 0; n-- {
		g.addExtraPhase(after, list, natural, trigger)
	}
	return nil
}

// addExtraPhase is PhaseHandler.addExtraPhase: each extra phase is followed
// by the next in list, the last by whatever already followed after (a
// previous insertion's first phase, with its trigger) or else next; after is
// then followed by list's first phase, which carries trigger.
func (g *Game) addExtraPhase(after PhaseType, list []PhaseType, next PhaseType, trigger *delayedTrigger) {
	for i, extra := range list {
		if i < len(list)-1 {
			g.extraPhases[extra] = append(g.extraPhases[extra], extraPhase{phase: list[i+1]})
			continue
		}
		if st := g.extraPhases[after]; len(st) > 0 {
			g.extraPhases[extra] = append(g.extraPhases[extra], st[len(st)-1])
			g.extraPhases[after] = st[:len(st)-1]
		} else {
			g.extraPhases[extra] = append(g.extraPhases[extra], extraPhase{phase: next})
		}
	}
	g.extraPhases[after] = append(g.extraPhases[after], extraPhase{phase: list[0], trigger: trigger})
}

// beforeFirstPostCombatMainEnd is PhaseHandler.beforeFirstPostCombatMainEnd
// (PhaseHandler.java:1000-1002): at most one main begun, or two while the
// current phase is a second main.
func (g *Game) beforeFirstPostCombatMainEnd() bool {
	limit := 1
	if g.activePhase == Main2 {
		limit = 2
	}
	return g.mainsThisTurn <= limit
}
