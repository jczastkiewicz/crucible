package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
)

// SetLife Redistribute$ (Reverse the Sands): the activator picks any number of
// players, then names each one's new life total from the totals still
// unassigned (LifeSetEffect.java:61-93). With 20 and 8 life, handing the
// activator the 8 gives the opponent the 20.
func TestSetLifeRedistributeSwapsTheChosenTotals(t *testing.T) {
	t.Parallel()

	g, p, other := newTwoPlayerGame(t)
	g.Player(p).Life, g.Player(other).Life = 20, 8
	c := engine.NewScriptedController()
	c.QueueEntityChoice([]engine.EntityID{engine.PlayerEntity(p), engine.PlayerEntity(other)})
	c.QueueNumberChoice(8)  // the activator's new total
	c.QueueNumberChoice(20) // the opponent gets what is left
	resolveLine(t, g, p, c, "DB$ SetLife | PlayerChoices$ Player | ChoiceAmount$ Any | Redistribute$ True")
	if got := g.Player(p).Life; got != 8 {
		t.Errorf("activator's life = %d, want 8", got)
	}
	if got := g.Player(other).Life; got != 20 {
		t.Errorf("opponent's life = %d, want 20", got)
	}
}

// An answer that is not one of the unassigned totals is refused, and choosing
// no players changes nothing.
func TestSetLifeRedistributeRefusesAnIllegalTotalAndAcceptsNoPlayers(t *testing.T) {
	t.Parallel()

	g, p, other := newTwoPlayerGame(t)
	g.Player(p).Life, g.Player(other).Life = 20, 8
	c := engine.NewScriptedController()
	c.QueueEntityChoice(nil)
	resolveLine(t, g, p, c, "DB$ SetLife | PlayerChoices$ Player | ChoiceAmount$ Any | Redistribute$ True")
	if g.Player(p).Life != 20 || g.Player(other).Life != 8 {
		t.Errorf("no players chosen, life = %d and %d, want 20 and 8", g.Player(p).Life, g.Player(other).Life)
	}
}
