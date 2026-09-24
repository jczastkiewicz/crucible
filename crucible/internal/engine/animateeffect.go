package engine

import "fmt"

// animateEffect is AnimateEffect.java: each targeted or Defined$ card
// (default Self) on the battlefield takes buildAnimate's characteristics
// until end of turn, or for good with Duration$ Permanent.
// RememberAnimated$ remembers each one on the host.
type animateEffect struct{}

func (animateEffect) Resolve(g *Game, a *Ability, _ PlayerController) error {
	template, err := buildAnimate(g, a, "Animate")
	if err != nil {
		return err
	}
	source := g.Card(a.Source)
	if !subAbilityConditionMet(g, source, a.Amounts, a.Params) {
		return nil
	}
	cards, err := targetedOrDefinedCards(source, a.Params, a.refs())
	if err != nil {
		return fmt.Errorf("engine: Animate: %w", err)
	}
	if hasParam(a, "RememberAnimated") {
		for _, id := range cards {
			if g.Card(id).Zone == Battlefield {
				source.Memory.Remember(CardEntity(id))
			}
		}
	}
	if template.empty() {
		return nil
	}
	g.animateCards(template, cards)
	return nil
}
