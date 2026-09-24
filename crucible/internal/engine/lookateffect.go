package engine

import "fmt"

// lookAtEffect is LookAtEffect.java: the activator looks at the targeted or
// Defined$ cards (GameAction.revealTo). The engine is omniscient -- no
// player-visibility state exists to update -- so resolving it changes
// nothing; it still resolves the card list so an unresolvable Defined$ fails
// closed rather than passing silently.
type lookAtEffect struct{}

func (lookAtEffect) Resolve(g *Game, a *Ability, _ PlayerController) error {
	for _, key := range [...]string{"Condition", "ConditionDefined"} {
		if _, ok := a.Params.Param(key); ok {
			return fmt.Errorf("engine: LookAt: %s$ not resolvable yet", key)
		}
	}
	source := g.Card(a.Source)
	if !subAbilityConditionMet(g, source, a.Amounts, a.Params) {
		return nil
	}
	if _, err := targetedOrDefinedCards(source, a.Params, a.Targets); err != nil {
		return fmt.Errorf("engine: LookAt: %w", err)
	}
	return nil
}
