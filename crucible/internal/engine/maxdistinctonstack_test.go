package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
)

// Count$MaxDistinctOnStack (Geometric Weird) is the turn's high-water mark of
// spells plus distinct non-spell sources on the stack, read after every push
// (MagicStack.java:324-334), not the stack as it stands: three abilities from
// two sources make 2, and the mark stays once they have resolved.
func TestMaxDistinctOnStackIsTheTurnsHighWaterMark(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGame(t)
	g.SetTurnState(1, p, engine.Main1)
	ping := scriptDef(t, "Test Pinger", "Artifact", "A:AB$ GainLife | Cost$ 0 | Defined$ You | LifeAmount$ 1")
	a := g.NewCard(ping, p, engine.Battlefield)
	b := g.NewCard(ping, p, engine.Battlefield)
	c := engine.NewScriptedController()
	for _, src := range []engine.CardID{a, a, b} {
		if !g.ActivateAbility(p, src, 0, c) {
			t.Fatal("ActivateAbility failed")
		}
	}
	if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	before := g.Player(p).Life
	resolveLine(t, g, p, c, "DB$ GainLife | Defined$ You | LifeAmount$ Count$MaxDistinctOnStack")
	if got := g.Player(p).Life - before; got != 2 {
		t.Errorf("Count$MaxDistinctOnStack = %d, want 2 (two distinct sources)", got)
	}
}
