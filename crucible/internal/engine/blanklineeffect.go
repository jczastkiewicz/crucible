package engine

// blankLineEffect is BlankLineEffect.java: an effect with no resolve of its
// own (SpellAbilityEffect.resolve is empty) that exists so a card's text
// lays out well. It still runs its SubAbility$ chain through Registry.
type blankLineEffect struct{}

func (blankLineEffect) Resolve(*Game, *Ability, PlayerController) error { return nil }
