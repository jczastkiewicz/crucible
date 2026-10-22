package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
)

// RemovePhase$ puts one delayed trigger per counter unit that was put, each
// removing a counter again in that phase (CountersPutEffect.java:663-693):
// Shambling Swarm's -1/-1 counters last until the end step.
func TestPutCounterRemovePhaseRemovesEachCounterAtThatPhase(t *testing.T) {
	t.Parallel()
	g, p, other := newTwoPlayerGame(t)
	libraryCards(t, g, p, 5)
	libraryCards(t, g, other, 5)
	bear := g.NewCard(creatureDefPT(t, "4", "4"), p, engine.Battlefield)
	c := engine.NewScriptedController()
	c.QueueTargets([]engine.EntityID{engine.CardEntity(bear)})
	def := etbChainDef(t, "Test Remove Phase",
		"DB$ PutCounter | ValidTgts$ Creature | CounterType$ M1M1 | CounterNum$ 2 | RemovePhase$ End of Turn")
	if _, err := castETBChain(t, g, p, def, c); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if n := g.Card(bear).Counters.Count(engine.M1M1); n != 2 {
		t.Fatalf("M1M1 before the end step = %d, want 2", n)
	}
	advanceUntil(t, g, c, func() bool { return g.ActivePhase() == engine.EndOfTurn })
	if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
		t.Fatal(err)
	}
	if n := g.Card(bear).Counters.Count(engine.M1M1); n != 0 {
		t.Errorf("M1M1 after the end step = %d, want 0", n)
	}
}
