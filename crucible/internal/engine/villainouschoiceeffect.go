package engine

import "fmt"

// villainousChoiceEffect is VillainousChoiceEffect.java: each Defined$ or
// targeted player faces a villainous choice -- picks Amount$ (default 1)
// of the Choices$ abilities -- and each pick resolves with that player
// remembered by the host. A player facing extra villainous choices (The
// Valeyard's AdditionalVillainousChoice$ static) is not modeled, so the
// effect fails while any such static is on the battlefield.
type villainousChoiceEffect struct{}

func (villainousChoiceEffect) Resolve(g *Game, a *Ability, controller PlayerController) error {
	if err := rejectParams(a, "VillainousChoice", "Condition"); err != nil {
		return err
	}
	source := g.Card(a.Source)
	if !subAbilityConditionMet(g, source, a.Amounts, a.Params) {
		return nil
	}
	if battlefieldStaticNames(g, "AdditionalVillainousChoice") {
		return fmt.Errorf("engine: VillainousChoice: AdditionalVillainousChoice$ statics not resolvable yet")
	}
	amount, err := optionalAmount(g, a, "VillainousChoice", "Amount", 1)
	if err != nil {
		return err
	}
	choices := additionalAbilities(a.Params, "Choices")
	names := make([]string, len(choices))
	all := make([]int, len(choices))
	for i, c := range choices {
		names[i], all[i] = c.SVar, i
	}
	if amount > len(choices) {
		amount = len(choices)
	}
	players, err := targetedOrDefinedPlayers(g, a.Controller, a.Source, a.Params, a.refs())
	if err != nil {
		return fmt.Errorf("engine: VillainousChoice: %w", err)
	}
	for _, p := range players {
		if len(choices) == 0 {
			continue
		}
		picked := controller.ChooseAbilitiesForEffect(g, p, a.Source, names, amount)
		if err := checkChoice(picked, all, amount, amount); err != nil {
			return fmt.Errorf("engine: VillainousChoice: %w", err)
		}
		for _, i := range picked {
			added := rememberAll(&source.Memory, []EntityID{PlayerEntity(p)})
			if err := g.resolveAdditional(a, controller, choices[i]); err != nil {
				return err
			}
			forgetAll(&source.Memory, added)
		}
	}
	return nil
}
