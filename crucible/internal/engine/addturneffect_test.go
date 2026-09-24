package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
)

// TestAddTurnEffectTakesExtraTurnThenResumes proves AddTurn: the next turn
// is the extra one, and turn order resumes normally after it.
func TestAddTurnEffectTakesExtraTurnThenResumes(t *testing.T) {
	t.Parallel()

	g, p, other := newTwoPlayerGame(t)
	c := engine.NewScriptedController()
	if _, err := castETBChain(t, g, p, etbChainDef(t, "Test Time Walk", "DB$ AddTurn | Defined$ You | NumTurns$ 1"), c); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	g.SetTurnState(1, p, engine.Cleanup)
	g.AdvancePhase(c)
	if got := g.ActivePlayer(); got != p {
		t.Fatalf("turn 2 active = %v, want %v (extra turn)", got, p)
	}
	g.SetTurnState(2, p, engine.Cleanup)
	g.AdvancePhase(c)
	if got := g.ActivePlayer(); got != other {
		t.Errorf("turn 3 active = %v, want %v", got, other)
	}
}

// TestSkipTurnEffectSkipsNextTurn proves SkipTurn: the skipped player's
// turn is passed over, once.
func TestSkipTurnEffectSkipsNextTurn(t *testing.T) {
	t.Parallel()

	g, p, other := newTwoPlayerGame(t)
	c := engine.NewScriptedController()
	if _, err := castETBChain(t, g, p, etbChainDef(t, "Test Skip", "DB$ SkipTurn | Defined$ Opponent | NumTurns$ 1"), c); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	g.SetTurnState(1, p, engine.Cleanup)
	g.AdvancePhase(c)
	if got := g.ActivePlayer(); got != p {
		t.Fatalf("next active = %v, want %v (opponent skipped)", got, p)
	}
	g.SetTurnState(2, p, engine.Cleanup)
	g.AdvancePhase(c)
	if got := g.ActivePlayer(); got != other {
		t.Errorf("following active = %v, want %v", got, other)
	}
}

// TestAddTurnEffectRejectsUnresolvedParam proves SkipUntap$ fails closed.
func TestAddTurnEffectRejectsUnresolvedParam(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGame(t)
	def := etbChainDef(t, "Test Time Walk Bad", "DB$ AddTurn | Defined$ You | NumTurns$ 1 | SkipUntap$ True")
	if _, err := castETBChain(t, g, p, def, engine.NewScriptedController()); err == nil {
		t.Fatal("ResolveStack succeeded, want an error for unresolved SkipUntap$")
	}
}
