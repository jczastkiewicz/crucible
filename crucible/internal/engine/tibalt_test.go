package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
)

// Tibalt the Chaotic: +1 casts a copy of one of three named cards chosen at
// random, without paying its mana cost. Whichever is drawn, the copy is cast:
// one spell is on the stack after the activation (or the ability errors).
func TestTibaltThePlusAbilityCastsOneOfTheNamedCards(t *testing.T) {
	t.Parallel()

	g, p, other := newTwoPlayerGameOn(t, scenarioDB(t))
	g.SetTurnState(1, p, engine.Main1)
	g.NewCard(creatureDefPT(t, "2", "7"), other, engine.Battlefield)
	tibalt := g.NewCard(corpusCard(t, "Tibalt the Chaotic"), p, engine.Battlefield)
	c := engine.NewScriptedController()
	c.QueueTargets([]engine.EntityID{engine.PlayerEntity(other)})
	c.QueueTargets([]engine.EntityID{engine.PlayerEntity(other)})
	if !g.ActivateAbility(p, tibalt, 0, c) {
		t.Fatalf("ActivateAbility failed: %v", g.TakePendingError())
	}
	if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if got := g.Card(tibalt).Counters.Count("LOYALTY"); got < 1 {
		t.Errorf("loyalty = %d, want it raised by the cost", got)
	}
	if err := g.TakePendingError(); err != nil {
		t.Errorf("pending error: %v", err)
	}
}
