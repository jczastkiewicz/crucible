package engine

// reverseTurnOrderEffect is ReverseTurnOrderEffect.java: Game.
// reverseTurnOrder flips turn order to the other direction (CR 101.4
// "turn order ... reversed"), read by nextPlayerAfter.
type reverseTurnOrderEffect struct{}

func (reverseTurnOrderEffect) Resolve(g *Game, a *Ability, _ PlayerController) error {
	if err := rejectParams(a, "ReverseTurnOrder", "Condition"); err != nil {
		return err
	}
	if !subAbilityConditionMet(g, g.Card(a.Source), a.Amounts, a.Params) {
		return nil
	}
	g.turnOrderReversed = !g.turnOrderReversed
	return nil
}
