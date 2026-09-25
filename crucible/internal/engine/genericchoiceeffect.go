package engine

//enginelint:allow id card game ability defined condition control amount parts additional effecthelpers

import "fmt"

// genericChoiceUnresolvedParams are ChooseGenericEffect.java's params this
// port cannot honour yet: random picks (AtRandom$, NumRandomChoices$), a
// hidden or recorded choice (Secretly$, SetChosenMode$, Guess$), and the
// batched damage/zone tables (DamageMap$, ChangeZoneTable$).
var genericChoiceUnresolvedParams = [...]string{
	"AtRandom", "NumRandomChoices", "Secretly", "SetChosenMode", "Guess",
	"DamageMap", "ChangeZoneTable", "LockInText",
	"Condition", "ConditionDefined", "Ultimate",
}

// genericChoiceEffect is ChooseGenericEffect.java: each Defined$/ValidTgts$
// player (default You) picks ChoiceAmount$ (default 1) of the Choices$
// abilities (ChooseAbilitiesForEffect) and each chosen one resolves in
// order. TempRemember$ swaps the chooser in as the host's only remembered
// player for the duration, as Java's own oldRem swap does.
//
// Java first drops choices whose restrictions fail or whose UnlessCost$
// cannot be paid, falling back to FallbackAbility$ when none remain; a
// choice carrying UnlessCost$ is rejected here instead of guessing that
// payability check.
type genericChoiceEffect struct{}

func (genericChoiceEffect) Resolve(g *Game, a *Ability, controller PlayerController) error {
	for _, key := range genericChoiceUnresolvedParams {
		if _, ok := a.Params.Param(key); ok {
			return fmt.Errorf("engine: GenericChoice: %s$ not resolvable yet", key)
		}
	}
	source := g.Card(a.Source)
	if !subAbilityConditionMet(g, source, a.Amounts, a.Params) {
		return nil
	}
	choices := additionalAbilities(a.Params, "Choices")
	names := make([]string, len(choices))
	for i, c := range choices {
		if _, ok := c.Ability.Param("UnlessCost"); ok {
			return fmt.Errorf("engine: GenericChoice: choice %s carries UnlessCost$, not resolvable yet", c.SVar)
		}
		names[i] = c.SVar
	}
	amountRaw, ok := a.Params.Param("ChoiceAmount")
	if !ok {
		amountRaw = "1"
	}
	amount, ok := resolveNamedAmount(g, a.Amounts, source, amountRaw)
	if !ok {
		return fmt.Errorf("engine: GenericChoice: ChoiceAmount$ %q not resolvable yet", amountRaw)
	}
	if amount > len(choices) {
		amount = len(choices)
	}
	players, err := targetedOrDefinedPlayers(g, a.Controller, a.Source, a.Params, a.refs())
	if err != nil {
		return fmt.Errorf("engine: GenericChoice: %w", err)
	}
	_, tempRem := a.Params.Param("TempRemember")
	for _, p := range players {
		if len(choices) == 0 {
			continue
		}
		picked := controller.ChooseAbilitiesForEffect(g, p, a.Source, names, amount)
		all := make([]int, len(choices))
		for i := range all {
			all[i] = i
		}
		if err := checkChoice(picked, all, amount, amount); err != nil {
			return fmt.Errorf("engine: GenericChoice: %w", err)
		}
		var oldRem []EntityID
		if tempRem {
			oldRem = swapRememberedPlayer(&source.Memory, p)
		}
		for _, i := range picked {
			if err := g.resolveAdditional(a, controller, choices[i]); err != nil {
				return err
			}
		}
		if tempRem {
			restoreRememberedPlayers(&source.Memory, p, oldRem)
		}
	}
	return nil
}
