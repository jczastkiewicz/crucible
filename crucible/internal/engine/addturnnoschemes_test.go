package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
)

// AddTurn NoSchemes$ (All in Good Time): during that extra turn no scheme is
// set in motion (the CantHappen SetInMotion effect of AddTurnEffect.java:69-80,
// Player.java:275-277), and the next turn sets them again.
func TestAddTurnNoSchemesStopsSettingSchemesInMotionForThatTurn(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGame(t)
	c := engine.NewScriptedController()
	g.SetTurnState(1, p, engine.Main2)
	scheme := g.NewCard(scriptDef(t, "Test Scheme", "Ongoing Scheme"), p, engine.SchemeDeck)
	resolveLine(t, g, p, c, "DB$ AddTurn | NumTurns$ 1 | NoSchemes$ True")
	startTurn := g.Turn()
	for g.Turn() == startTurn {
		g.AdvancePhase(c)
	}
	g.SetTurnState(g.Turn(), g.ActivePlayer(), engine.Main1)
	resolveLine(t, g, g.ActivePlayer(), c, "DB$ SetInMotion")
	if z := g.Card(scheme).Zone; z != engine.SchemeDeck {
		t.Fatalf("during the NoSchemes turn the scheme is in %v, want SchemeDeck", z)
	}
	next := g.Turn()
	for g.Turn() == next {
		g.AdvancePhase(c)
	}
	g.SetTurnState(g.Turn(), p, engine.Main1)
	resolveLine(t, g, p, c, "DB$ SetInMotion")
	if z := g.Card(scheme).Zone; z != engine.Command {
		t.Errorf("after the NoSchemes turn the scheme is in %v, want Command", z)
	}
}
