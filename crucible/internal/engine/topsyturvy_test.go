package engine_test

import (
	"slices"
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
	"github.com/jczastkiewicz/crucible/pkg/javarand"
)

// Topsy Turvy (Mode$ PhaseReversed): the groups of a turn run in reverse order
// (PhaseType.getNext, PhaseType.java:142-158), the steps inside one forward,
// and Cleanup stays last. Two sources cancel (StaticAbilityTurnPhaseReversed
// toggles per source).
func TestPhasesReversedUnderTopsyTurvy(t *testing.T) {
	t.Parallel()
	walk := func(g *engine.Game, c engine.PlayerController, from engine.PhaseType, n int) []engine.PhaseType {
		g.SetTurnState(2, g.Players()[0], from)
		var got []engine.PhaseType
		for range n {
			g.AdvancePhase(c)
			got = append(got, g.ActivePhase())
		}
		return got
	}
	t.Run("one source", func(t *testing.T) {
		t.Parallel()
		g, p, _ := newTwoPlayerGameOn(t, scenarioDB(t))
		g.NewCard(corpusCard(t, "Topsy Turvy"), p, engine.Battlefield)
		c := engine.NewScriptedController()
		got := walk(g, c, engine.Main2, 3)
		want := []engine.PhaseType{engine.CombatBegin, engine.DeclareAttackers, engine.DeclareBlockers}
		if !slices.Equal(got, want) {
			t.Errorf("after Main2: %v, want %v", got, want)
		}
		got = walk(g, c, engine.Draw, 1)
		if want := []engine.PhaseType{engine.Cleanup}; !slices.Equal(got, want) {
			t.Errorf("after Draw: %v, want %v", got, want)
		}
		turn := g.Turn()
		g.AdvancePhase(c)
		if g.ActivePhase() != engine.EndOfTurn || g.Turn() != turn+1 {
			t.Errorf("after Cleanup: %v turn %d, want the next turn to open at the end step", g.ActivePhase(), g.Turn())
		}
	})
	t.Run("two sources cancel", func(t *testing.T) {
		t.Parallel()
		g, p, _ := newTwoPlayerGameOn(t, scenarioDB(t))
		g.NewCard(corpusCard(t, "Topsy Turvy"), p, engine.Battlefield)
		g.NewCard(corpusCard(t, "Topsy Turvy"), p, engine.Battlefield)
		got := walk(g, engine.NewScriptedController(), engine.Main1, 1)
		if want := []engine.PhaseType{engine.CombatBegin}; !slices.Equal(got, want) {
			t.Errorf("after Main1: %v, want %v", got, want)
		}
	})
}

// Topsy Turvy's TurnReversed line holds with more than two players: the next
// turn then goes to the previous seat (Game.getTurnOrder, Game.java:551-556);
// with two players CheckSVar$ X GT2 fails and the order stays.
func TestTurnOrderReversedUnderTopsyTurvyWithThreePlayers(t *testing.T) {
	t.Parallel()
	next := func(names ...string) (engine.PlayerID, []engine.PlayerID) {
		g := engine.NewGame(scenarioDB(t), javarand.New(1), names)
		ids := g.Players()
		g.NewCard(corpusCard(t, "Topsy Turvy"), ids[0], engine.Battlefield)
		g.SetTurnState(1, ids[0], engine.Cleanup)
		g.AdvancePhase(engine.NewScriptedController())
		return g.ActivePlayer(), ids
	}
	if got, ids := next("a", "b", "c"); got != ids[2] {
		t.Errorf("three players: next turn is %v, want the previous seat %v", got, ids[2])
	}
	if got, ids := next("a", "b"); got != ids[1] {
		t.Errorf("two players: next turn is %v, want %v", got, ids[1])
	}
}
