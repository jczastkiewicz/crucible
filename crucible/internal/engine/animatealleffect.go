package engine

// animateAllEffect is AnimateAllEffect.java: every battlefield card matching
// ValidCards$ takes buildAnimate's characteristics. Zone$ past the
// battlefield and the player-scoped ValidTgts$/Defined$ form fail closed.
type animateAllEffect struct{}

func (animateAllEffect) Resolve(g *Game, a *Ability, _ PlayerController) error {
	if err := rejectParams(a, "AnimateAll", "Zone", "ValidTgts", "Defined", "RememberAnimated"); err != nil {
		return err
	}
	template, err := buildAnimate(g, a, "AnimateAll")
	if err != nil {
		return err
	}
	source := g.Card(a.Source)
	if !subAbilityConditionMet(g, source, a.Amounts, a.Params) {
		return nil
	}
	spec, _ := a.Params.Param("ValidCards")
	var cards []CardID
	for _, p := range g.Players() {
		cards = append(cards, g.Zone(Battlefield, p).Cards()...)
	}
	if spec != "" {
		cards = filterValid(g, cards, spec, a.Controller, a.Source)
	}
	if template.empty() {
		return nil
	}
	g.animateCards(template, cards)
	return nil
}
