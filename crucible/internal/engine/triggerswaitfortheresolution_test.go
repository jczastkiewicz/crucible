package engine_test

import (
	"slices"
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
)

// A trigger the resolution fires waits until the resolution and its
// state-based actions are done (CR 603.3, ADR-0064): a creature spell's
// enters trigger goes on the stack after the spell resolved, as Java's
// frozen stack has it (MagicStack.java:698-700), not inside the resolution.
func TestEntersTriggerGoesOnTheStackAfterTheSpellResolved(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGame(t)
	var sink recordingSink
	g.SetSink(&sink)
	host, err := castETBChain(t, g, p, etbChainDef(t, "Test Shape", "DB$ GainLife | Defined$ You | LifeAmount$ 1"), engine.NewScriptedController())
	if err != nil {
		t.Fatal(err)
	}
	var got []engine.EventKind
	for _, e := range sink.events {
		if e.Source == host && (e.Kind == engine.AbilityActivated || e.Kind == engine.AbilityResolved) {
			got = append(got, e.Kind)
		}
	}
	want := []engine.EventKind{engine.AbilityActivated, engine.AbilityResolved, engine.AbilityActivated, engine.AbilityResolved}
	if !slices.Equal(got, want) {
		t.Errorf("stack events %v, want spell pushed, spell resolved, trigger pushed, trigger resolved", got)
	}
	if g.Player(p).Life != 21 {
		t.Errorf("life %d, want 21", g.Player(p).Life)
	}
}
