package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
)

// TestLookAtEffectChangesNothing proves LookAt resolves with no state
// change (the engine is omniscient) and still fails closed on a Defined$
// it cannot resolve.
func TestLookAtEffectChangesNothing(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGame(t)
	c := engine.NewScriptedController()
	if _, err := castETBChain(t, g, p, etbChainDef(t, "Test Look", "DB$ LookAt | Defined$ Self"), c); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	g2, p2, _ := newTwoPlayerGame(t)
	if _, err := castETBChain(t, g2, p2, etbChainDef(t, "Test Look Bad", "DB$ LookAt | Defined$ TriggeredCard"), engine.NewScriptedController()); err == nil {
		t.Fatal("ResolveStack succeeded, want an error for an unresolvable Defined$")
	}
}
