// Per-card activation counts: Card.numberTurnActivations and
// numberGameActivations, the state ActivationLimit$ and GameActivationLimit$
// read (activateability.go's activationLimitsMet).

package engine

// activationCount is one A: line's activations, by its index in
// Faces[0].Abilities.
type activationCount struct {
	index      int
	turn, game int
}

// activationCounts is a card's activation table.
type activationCounts []activationCount

// of is how often the A: line at index was activated this turn and this game
// (Card.getAbilityActivatedThisTurn/ThisGame).
func (a activationCounts) of(index int) (turn, game int) {
	for _, c := range a {
		if c.index == index {
			return c.turn, c.game
		}
	}
	return 0, 0
}

// note is Card.addAbilityActivated, called as an activated ability goes on
// the stack (MagicStack.java:229,305).
func (a *activationCounts) note(index int) {
	for i := range *a {
		if (*a)[i].index == index {
			(*a)[i].turn++
			(*a)[i].game++
			return
		}
	}
	*a = append(*a, activationCount{index: index, turn: 1, game: 1})
}

// resetTurn is Card.resetActivationsPerTurn's activation tables.
func (a activationCounts) resetTurn() {
	for i := range a {
		a[i].turn = 0
	}
}

// clone is activationCounts' half of Game.Clone.
func (a activationCounts) clone() activationCounts { return append(activationCounts(nil), a...) }
