package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
)

// A match is first to N wins: games alternate, draws never end it, and the
// loser of the last game is the one who decides the next start.
func TestMatchSeriesScoreAndLoser(t *testing.T) {
	t.Parallel()

	m, err := engine.NewMatch(2, 2)
	if err != nil {
		t.Fatalf("NewMatch: %v", err)
	}
	if _, ok := m.LastLoser(); ok {
		t.Fatal("LastLoser before any game")
	}
	steps := []struct {
		o         engine.GameOutcome
		wantLoser engine.PlayerID
		wantOver  bool
	}{
		{engine.GameOutcome{Winner: 1, HasWinner: true}, 2, false},
		{engine.GameOutcome{}, 1, false}, // a draw: nobody won, the first seat decides
		{engine.GameOutcome{Winner: 2, HasWinner: true}, 1, false},
		{engine.GameOutcome{Winner: 2, HasWinner: true}, 1, true},
	}
	for i, s := range steps {
		if err := m.Record(s.o); err != nil {
			t.Fatalf("game %d: Record: %v", i, err)
		}
		if l, _ := m.LastLoser(); l != s.wantLoser {
			t.Errorf("game %d: loser = %d, want %d", i, l, s.wantLoser)
		}
		if m.IsOver() != s.wantOver {
			t.Errorf("game %d: over = %v, want %v", i, m.IsOver(), s.wantOver)
		}
	}
	if w, ok := m.Winner(); !ok || w != 2 {
		t.Errorf("Winner = %d, %v, want 2, true", w, ok)
	}
	if m.GamesWon(1) != 1 || m.GamesWon(2) != 2 || m.GamesPlayed() != 4 {
		t.Errorf("score = %d-%d over %d games", m.GamesWon(1), m.GamesWon(2), m.GamesPlayed())
	}
	if err := m.Record(engine.GameOutcome{Winner: 1, HasWinner: true}); err == nil {
		t.Error("Record into a finished match succeeded")
	}
}

func TestNewMatchRejectsDegenerateShapes(t *testing.T) {
	t.Parallel()

	for _, tc := range [][2]int{{1, 2}, {2, 0}} {
		if _, err := engine.NewMatch(tc[0], tc[1]); err == nil {
			t.Errorf("NewMatch(%d, %d) succeeded", tc[0], tc[1])
		}
	}
}

// Game one asks the coin-flip winner; a later game asks the last loser with
// isFirstGame false.
func TestDetermineFirstTurnPlayerAsksTheDecider(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	players := g.Players()
	r := &startingPlayerRecorder{ScriptedController: engine.NewScriptedController(), answerWith: players[1]}
	if got := engine.DetermineFirstTurnPlayer(g, r, nil, false); got != players[1] || !r.isFirstGame || r.asks != 1 {
		t.Errorf("game one: got %d first=%v asks=%d", got, r.isFirstGame, r.asks)
	}

	m, _ := engine.NewMatch(2, 2)
	_ = m.Record(engine.GameOutcome{Winner: players[1], HasWinner: true})
	r = &startingPlayerRecorder{ScriptedController: engine.NewScriptedController(), answerWith: players[0]}
	engine.DetermineFirstTurnPlayer(g, r, m, false)
	if r.decider != players[0] || r.isFirstGame {
		t.Errorf("game two: decider %d first=%v, want the loser %d and false", r.decider, r.isFirstGame, players[0])
	}
}

// Puzzle, Archenemy and Power Play name the starting player without asking.
func TestDetermineFirstTurnPlayerSpecialRules(t *testing.T) {
	t.Parallel()

	t.Run("puzzle is the first seat", func(t *testing.T) {
		t.Parallel()
		g := newGame(t, "a", "b")
		r := &startingPlayerRecorder{ScriptedController: engine.NewScriptedController(), answerWith: g.Players()[1]}
		if got := engine.DetermineFirstTurnPlayer(g, r, nil, true); got != g.Players()[0] || r.asks != 0 {
			t.Errorf("got %d asks=%d", got, r.asks)
		}
	})
	t.Run("the archenemy goes first even after losing", func(t *testing.T) {
		t.Parallel()
		g := newGame(t, "a", "b")
		players := g.Players()
		schemeDeck(t, g, players[1], 1)
		m, _ := engine.NewMatch(2, 2)
		_ = m.Record(engine.GameOutcome{Winner: players[0], HasWinner: true})
		r := &startingPlayerRecorder{ScriptedController: engine.NewScriptedController(), answerWith: players[0]}
		if got := engine.DetermineFirstTurnPlayer(g, r, m, false); got != players[1] || r.asks != 0 {
			t.Errorf("got %d asks=%d, want the archenemy %d unasked", got, r.asks, players[1])
		}
	})
	t.Run("power play owner goes first", func(t *testing.T) {
		t.Parallel()
		g := newGame(t, "a", "b")
		players := g.Players()
		g.NewCard(corpusCard(t, "Power Play"), players[1], engine.Command)
		r := &startingPlayerRecorder{ScriptedController: engine.NewScriptedController(), answerWith: players[0]}
		if got := engine.DetermineFirstTurnPlayer(g, r, nil, false); got != players[1] || r.asks != 0 {
			t.Errorf("got %d asks=%d, want %d unasked", got, r.asks, players[1])
		}
	})
	t.Run("two power plays pick one of their owners", func(t *testing.T) {
		t.Parallel()
		g := newGame(t, "a", "b")
		players := g.Players()
		for _, p := range players {
			g.NewCard(corpusCard(t, "Power Play"), p, engine.Command)
		}
		r := &startingPlayerRecorder{ScriptedController: engine.NewScriptedController()}
		got := engine.DetermineFirstTurnPlayer(g, r, nil, false)
		if (got != players[0] && got != players[1]) || r.asks != 0 {
			t.Errorf("got %d asks=%d", got, r.asks)
		}
	})
}
