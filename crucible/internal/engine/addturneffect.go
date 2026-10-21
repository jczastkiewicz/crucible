package engine

//enginelint:allow card game ability defined condition control amount additional delayedtrigger

import (
	"fmt"
	"strings"

	"github.com/jczastkiewicz/crucible/internal/carddb/compile"
)

// addTurnUnresolvedParams: a skipped untap step (SkipUntap$) or an Archenemy
// scheme restriction (NoSchemes$). ExtraTurnDelayedTrigger$ is read below.
var addTurnUnresolvedParams = [...]string{"SkipUntap", "NoSchemes", "Ultimate"}

// addTurnEffect is AddTurnEffect.java: each target player (default You)
// takes NumTurns$ extra turns after this one (Game.addExtraTurn, turn.go).
// Extra turns stack: the most recently added is taken first (CR 500.7).
type addTurnEffect struct{}

func (addTurnEffect) Resolve(g *Game, a *Ability, _ PlayerController) error {
	if err := rejectUnreadCondition(a, a.Params.Name); err != nil {
		return err
	}
	for _, key := range addTurnUnresolvedParams {
		if _, ok := a.Params.Param(key); ok {
			return fmt.Errorf("engine: AddTurn: %s$ not resolvable yet", key)
		}
	}
	source := g.Card(a.Source)
	if !subAbilityConditionMet(g, source, a.Amounts, a.Params) {
		return nil
	}
	raw, _ := a.Params.Param("NumTurns")
	n, ok := resolveNamedAmount(g, a.Amounts, source, raw)
	if !ok {
		return fmt.Errorf("engine: AddTurn: NumTurns$ %q not resolvable yet", raw)
	}
	players, err := targetedOrDefinedPlayers(g, a.Controller, a.Source, a.Params, a.refs())
	if err != nil {
		return fmt.Errorf("engine: AddTurn: %w", err)
	}
	var trigger *compile.Ability
	if subs := additionalAbilities(a.Params, "ExtraTurnDelayedTrigger"); len(subs) > 0 {
		trigger = subs[0].Ability
		if mode, _ := trigger.Param("Mode"); !strings.EqualFold(mode, "Phase") {
			return fmt.Errorf("engine: AddTurn: ExtraTurnDelayedTrigger$ Mode$ %q not resolvable yet", mode)
		}
	}
	for _, p := range players {
		for i := 0; i < n; i++ {
			g.addExtraTurn(p)
			if trigger != nil {
				// AddTurnEffect.java:50-58 hangs the trigger on the ExtraTurn
				// object. The extra turn follows this one at once, so a
				// NextTurn$-shaped delayed trigger (live for exactly the
				// next turn) stands in for it (quirks.md).
				g.delayed = append(g.delayed, delayedTrigger{
					Trigger: trigger, Host: a.Source, Controller: a.Controller, Amounts: a.Amounts,
					ThisTurn: true, AtCleanup: true, HostTransforms: source.Transforms,
				})
			}
		}
	}
	return nil
}
