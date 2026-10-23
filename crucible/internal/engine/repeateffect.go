package engine

//enginelint:allow card game ability condition control amount trigger presentcheck defined

import "fmt"

// repeatUnresolvedParams: none; RepeatOptionalDecider$ names the player asked
// "again?" (RepeatEffect.java:110-115).
var repeatUnresolvedParams = [...]string{}

// repeatEffect is RepeatEffect.java: RepeatSubAbility$ resolves at least
// once, then again while the repeat conditions hold -- RepeatPresent$/
// RepeatCompare$ over the battlefield or the RepeatDefined$ cards, RepeatCheckSVar$/RepeatSVarCompare$,
// and RepeatOptional$ asking the activator (ConfirmEffect) -- stopping at
// MaxRepeat$ or when the game ends. 37 real lines.
type repeatEffect struct{}

func (repeatEffect) Resolve(g *Game, a *Ability, controller PlayerController) error {
	if err := rejectUnreadCondition(a, a.Params.Name); err != nil {
		return err
	}
	for _, key := range repeatUnresolvedParams {
		if _, ok := a.Params.Param(key); ok {
			return fmt.Errorf("engine: Repeat: %s$ not resolvable yet", key)
		}
	}
	source := g.Card(a.Source)
	if !subAbilityConditionMet(g, source, a.Amounts, a.Params) {
		return nil
	}
	// RepeatCheckSVar$ is first read after the first repetition
	// (RepeatEffect.java:39-52, checkRepeatConditions), by which time the
	// repeated chain has usually stored it (StoreSVar, Mana Clash and Sword
	// of Dungeons & Dragons): it is not read up front. One that does not
	// resolve then is a pending error (checkSVarMatches).
	maxRepeat := -1
	if raw, ok := a.Params.Param("MaxRepeat"); ok {
		n, ok := resolveNamedAmount(g, a.Amounts, source, raw)
		if !ok {
			return fmt.Errorf("engine: Repeat: MaxRepeat$ %q not resolvable yet", raw)
		}
		if n == 0 {
			return nil
		}
		maxRepeat = n
	}
	for count := 1; ; count++ {
		if err := g.resolveAdditionalKey(a, controller, "RepeatSubAbility"); err != nil {
			return err
		}
		if maxRepeat >= 0 && count >= maxRepeat {
			return nil
		}
		if g.over ||
			!isPresentMatches(g, source, a.Amounts, a.Params, "RepeatPresent", "RepeatCompare", "RepeatDefined", "", "") ||
			!checkSVarMatches(g, source, a.Amounts, a.Params, "RepeatCheckSVar", "RepeatSVarCompare", "") {
			return nil
		}
		if _, ok := a.Params.Param("RepeatOptional"); ok {
			// RepeatEffect.java:110-115: the first RepeatOptionalDecider$ player,
			// else the activator, confirms.
			decider := a.Controller
			if raw, ok := a.Params.Param("RepeatOptionalDecider"); ok {
				players, err := definedPlayers(g, a.Controller, a.Source, raw, a.refs())
				if err != nil {
					return fmt.Errorf("engine: Repeat: RepeatOptionalDecider$: %w", err)
				}
				if len(players) == 0 {
					// Java's getDefinedPlayers(...).get(0) throws here.
					return fmt.Errorf("engine: Repeat: RepeatOptionalDecider$ %q names no player", raw)
				}
				decider = players[0]
			}
			if !controller.ConfirmEffect(g, decider, a.Source) {
				return nil
			}
		}
	}
}
