package engine

import (
	"fmt"
	"strings"

	"github.com/jczastkiewicz/crucible/internal/mana"
)

// chooseColorUnresolvedParams are ChooseColorEffect.java's params this port
// cannot honour yet: Random$ (Aggregates.random), ColorsFrom$ (the colors of
// a defined card set), and the bare CheckSVar$/SVarCompare$ activation gate
// on 1 line each.
var chooseColorUnresolvedParams = [...]string{
	"Random", "ColorsFrom", "CheckSVar", "SVarCompare",
	"Condition", "ConditionDefined", "SorcerySpeed", "InstantSpeed", "Forecast",
}

// chooseColorEffect is ChooseColorEffect.java: each chooser (Defined$,
// default You) picks colors out of all five, or Choices$'s restricted list,
// minus Exclude$. The count is one, exactly two for TwoColors$, one or more
// for OrColors$, and UpTo$ lowers the minimum to zero -- Java's own
// cntMin/cntMax pair. A colorless answer ends the effect without recording
// anything, as Java's own early return does. 122 real lines.
type chooseColorEffect struct{}

func (chooseColorEffect) Resolve(g *Game, a *Ability, controller PlayerController) error {
	for _, key := range chooseColorUnresolvedParams {
		if _, ok := a.Params.Param(key); ok {
			return fmt.Errorf("engine: ChooseColor: %s$ not resolvable yet", key)
		}
	}
	source := g.Card(a.Source)
	if !subAbilityConditionMet(g, source, a.Amounts, a.Params) {
		return nil
	}
	options := mana.AllColors
	if raw, ok := a.Params.Param("Choices"); ok {
		c, err := parseColorNames(raw)
		if err != nil {
			return fmt.Errorf("engine: ChooseColor: Choices$: %w", err)
		}
		options = c
	}
	if raw, ok := a.Params.Param("Exclude"); ok {
		c, err := parseColorNames(raw)
		if err != nil {
			return fmt.Errorf("engine: ChooseColor: Exclude$: %w", err)
		}
		options &^= c
	}
	_, upTo := a.Params.Param("UpTo")
	_, two := a.Params.Param("TwoColors")
	_, or := a.Params.Param("OrColors")
	lo, hi := 1, 1
	switch {
	case upTo:
		lo = 0
	case two:
		lo = 2
	}
	switch {
	case two:
		hi = 2
	case or:
		hi = options.Count()
	}

	choosers, err := targetedOrDefinedPlayers(g, a.Controller, a.Source, a.Params, a.refs())
	if err != nil {
		return fmt.Errorf("engine: ChooseColor: %w", err)
	}
	for _, pid := range choosers {
		chosen := controller.ChooseColors(g, pid, a.Source, options, lo, hi)
		if chosen&^options != 0 || chosen.Count() < lo || chosen.Count() > hi {
			return fmt.Errorf("engine: ChooseColor: controller chose %v, want %d to %d of %v", chosen, lo, hi, options)
		}
		if chosen.IsColorless() {
			return nil
		}
		source.Memory.SetChosenColors(chosen)
	}
	return nil
}

// parseColorNames reads a comma-separated list of MagicColor's own
// lowercase color names, what Choices$ and Exclude$ spell ("white,blue").
func parseColorNames(raw string) (mana.Colors, error) {
	var out mana.Colors
	for _, name := range strings.Split(raw, ",") {
		switch strings.ToLower(strings.TrimSpace(name)) {
		case "white":
			out |= mana.White
		case "blue":
			out |= mana.Blue
		case "black":
			out |= mana.Black
		case "red":
			out |= mana.Red
		case "green":
			out |= mana.Green
		default:
			return 0, fmt.Errorf("color %q not resolvable yet", name)
		}
	}
	return out, nil
}
