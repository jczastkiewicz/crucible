// Charm: a modal ability (CR 700.2). Its modes are chosen as the ability
// is put on the stack, each chosen mode's targets with them, and resolving
// the Charm resolves the chosen modes in printed order.
//
// Ported from forge-game/src/main/java/forge/game/ability/effects/
// CharmEffect.java (makeChoices, makePossibleOptions, chainAbilities).
// Java chains the chosen modes onto the Charm as sub-abilities; this port
// keeps them on Ability.Modes instead, since a compiled Ability is
// immutable (PORT-2).

package engine

import (
	"fmt"
	"sort"
	"strings"
)

// charmUnresolvedParams are CharmEffect params this port does not model:
// ChoiceRestriction$ (Card.getChosenModes' once-per-turn/-game bookkeeping),
// CanRepeatModes$, Optional$, Chooser$ (an opponent chooses), Random$
// Compare, Defined$ (never read by CharmEffect; its six lines lean on a
// mode reading it), and the activation-limit params whose restriction the
// port's activation path does not enforce.
var charmUnresolvedParams = [...]string{
	"ChoiceRestriction", "CanRepeatModes", "Optional", "Chooser", "RandomCompare",
	"RandomCompareSVar", "Defined", "GameActivationLimit", "ActivationLimit",
}

// chooseCharmModes is makeChoices, run from pushTriggeredAbilities before
// the Charm goes on the stack. It fills a.Modes and reports whether the
// Charm may go on the stack: false when fewer modes are legal than
// MinCharmNum$ demands, or -- for a trigger -- when no mode was chosen
// ("trigger without chosen modes are removed from stack"). An activated
// Charm reaches here through the same push, and Java lets it keep zero
// modes when MinCharmNum$ allows; this port treats both the same, since
// neither can resolve to anything.
func (g *Game) chooseCharmModes(controller PlayerController, a *Ability) (bool, error) {
	for _, key := range charmUnresolvedParams {
		if _, ok := a.Params.Param(key); ok {
			return false, fmt.Errorf("engine: Charm: %s$ not resolvable yet", key)
		}
	}
	host := g.Card(a.Source)

	var options []Ability
	var names []string
	for _, sub := range a.Params.Subs {
		if !strings.EqualFold(sub.Key, "Choices") {
			continue
		}
		api, ok := APIByName(sub.Ability.Name)
		if !ok {
			return false, fmt.Errorf("engine: Charm: mode %s: unrecognized API %q", sub.SVar, sub.Ability.Name)
		}
		mode := Ability{
			API: api, Source: a.Source, Controller: a.Controller, Params: sub.Ability,
			Amounts: a.Amounts, TriggerRemembered: a.TriggerRemembered,
		}
		if !g.modeHasLegalTargets(&mode) {
			continue
		}
		options = append(options, mode)
		names = append(names, sub.SVar)
	}

	num := 1
	if raw, ok := a.Params.Param("CharmNum"); ok {
		n, ok := resolveNamedAmount(g, a.Amounts, host, raw)
		if !ok {
			return false, fmt.Errorf("engine: Charm: CharmNum$ %q is not resolvable", raw)
		}
		num = n
	}
	min := num
	if raw, ok := a.Params.Param("MinCharmNum"); ok {
		n, ok := resolveNamedAmount(g, a.Amounts, host, raw)
		if !ok {
			return false, fmt.Errorf("engine: Charm: MinCharmNum$ %q is not resolvable", raw)
		}
		min = n
	}
	if min > len(options) {
		return false, nil
	}
	if num > len(options) {
		num = len(options)
	}

	var chosen []int
	if random, ok := a.Params.Param("Random"); ok {
		if random != "True" {
			return false, fmt.Errorf("engine: Charm: Random$ %q not resolvable yet", random)
		}
		chosen = g.randomSample(len(options), num)
	} else {
		chosen = controller.ChooseModesForAbility(g, a.Controller, a.Source, names, min, num)
		if err := checkModeChoice(chosen, len(options), min, num); err != nil {
			return false, err
		}
	}
	sort.Ints(chosen)

	a.Modes = nil
	for _, i := range chosen {
		mode := options[i]
		if !g.resolveTargets(controller, &mode) {
			return false, nil
		}
		a.Modes = append(a.Modes, mode)
	}
	return len(a.Modes) > 0, nil
}

// modeHasLegalTargets is makePossibleOptions' CR 603.3c filter: a mode
// that targets, needs at least one target and has no candidate cannot be
// chosen.
func (g *Game) modeHasLegalTargets(mode *Ability) bool {
	validTgts, ok := mode.Params.Param("ValidTgts")
	if !ok {
		return true
	}
	minStr, ok := mode.Params.Param("TargetMin")
	if !ok {
		minStr = "1"
	}
	targetMin, ok := resolveNamedAmount(g, mode.Amounts, g.Card(mode.Source), minStr)
	if !ok || targetMin == 0 {
		return ok
	}
	return len(g.targetCandidates(mode.Controller, mode.Source, validTgts)) > 0
}

// checkModeChoice rejects a controller answer that is not min..max distinct
// indices into n options.
func checkModeChoice(chosen []int, n, min, max int) error {
	if len(chosen) < min || len(chosen) > max {
		return fmt.Errorf("engine: Charm: chose %d modes, want %d..%d", len(chosen), min, max)
	}
	seen := make([]bool, n)
	for _, i := range chosen {
		if i < 0 || i >= n || seen[i] {
			return fmt.Errorf("engine: Charm: mode index %d is not a distinct choice among %d", i, n)
		}
		seen[i] = true
	}
	return nil
}

// charmEffect resolves the modes chooseCharmModes picked, in printed
// order, each through the Registry driving the stack -- its own
// SubAbility$ chain and UnlessCost$ included.
type charmEffect struct{}

func (charmEffect) Resolve(g *Game, a *Ability, controller PlayerController) error {
	if a.modesErr != nil {
		return a.modesErr
	}
	if a.Modes == nil {
		return fmt.Errorf("engine: Charm: no modes were chosen when it was put on the stack")
	}
	for _, mode := range a.Modes {
		m := mode
		m.hostTransforms, m.hasHostTransforms = a.hostTransforms, a.hasHostTransforms
		m.damageMap = a.damageMap
		r := g.registry
		err := r.Resolve(g, &m, controller)
		g.registry = r
		if err != nil {
			return err
		}
	}
	return nil
}
