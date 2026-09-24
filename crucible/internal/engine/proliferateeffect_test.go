package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
)

// TestProliferateEffectAddsOneOfEachKind proves CR 701.34a: every chosen
// permanent or player gets one more of each counter kind it already has,
// and an unchosen one is untouched.
func TestProliferateEffectAddsOneOfEachKind(t *testing.T) {
	t.Parallel()

	g, p, other := newTwoPlayerGame(t)
	mine := g.NewCard(creatureDefPT(t, "2", "2"), p, engine.Battlefield)
	g.Card(mine).Counters.Add(engine.P1P1, 1)
	g.Card(mine).Counters.Add(engine.Charge, 2)
	theirs := g.NewCard(creatureDefPT(t, "2", "2"), other, engine.Battlefield)
	g.Card(theirs).Counters.Add(engine.P1P1, 1)
	g.Player(other).Counters.Add(engine.Poison, 1)

	c := engine.NewScriptedController()
	c.QueueEntityChoice([]engine.EntityID{engine.PlayerEntity(other), engine.CardEntity(mine)})
	if _, err := castETBChain(t, g, p, etbChainDef(t, "Test Proliferate", "DB$ Proliferate"), c); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if got := g.Card(mine).Counters.Count(engine.P1P1); got != 2 {
		t.Errorf("mine P1P1 = %d, want 2", got)
	}
	if got := g.Card(mine).Counters.Count(engine.Charge); got != 3 {
		t.Errorf("mine CHARGE = %d, want 3", got)
	}
	if got := g.Player(other).Counters.Count(engine.Poison); got != 2 {
		t.Errorf("opponent POISON = %d, want 2", got)
	}
	if got := g.Card(theirs).Counters.Count(engine.P1P1); got != 1 {
		t.Errorf("unchosen P1P1 = %d, want 1", got)
	}
}

// TestProliferateEffectAmountRepeats proves Amount$ runs the whole choice
// that many times.
func TestProliferateEffectAmountRepeats(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGame(t)
	mine := g.NewCard(creatureDefPT(t, "2", "2"), p, engine.Battlefield)
	g.Card(mine).Counters.Add(engine.P1P1, 1)

	c := engine.NewScriptedController()
	c.QueueEntityChoice([]engine.EntityID{engine.CardEntity(mine)})
	c.QueueEntityChoice([]engine.EntityID{engine.CardEntity(mine)})
	if _, err := castETBChain(t, g, p, etbChainDef(t, "Test Proliferate Twice", "DB$ Proliferate | Amount$ 2"), c); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if got := g.Card(mine).Counters.Count(engine.P1P1); got != 3 {
		t.Errorf("P1P1 = %d, want 3", got)
	}
}

// TestProliferateEffectRejectsEntityWithoutCounters proves only entities
// that already have a counter are offered (GO-7).
func TestProliferateEffectRejectsEntityWithoutCounters(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGame(t)
	bare := g.NewCard(creatureDefPT(t, "2", "2"), p, engine.Battlefield)

	c := engine.NewScriptedController()
	c.QueueEntityChoice([]engine.EntityID{engine.CardEntity(bare)})
	if _, err := castETBChain(t, g, p, etbChainDef(t, "Test Proliferate Bad", "DB$ Proliferate"), c); err == nil {
		t.Fatal("ResolveStack succeeded, want an error for a permanent with no counters")
	}
}
