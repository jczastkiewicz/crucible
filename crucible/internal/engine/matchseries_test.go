package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
)

// startingPlayerRecorder records the ChooseStartingPlayer ask.
type startingPlayerRecorder struct {
	*engine.ScriptedController
	decider      engine.PlayerID
	isFirstGame  bool
	asks         int
	answerWith   engine.PlayerID
	hasAnswerSet bool
}

func (r *startingPlayerRecorder) ChooseStartingPlayer(_ *engine.Game, decider engine.PlayerID, isFirstGame bool) engine.PlayerID {
	r.asks++
	r.decider, r.isFirstGame = decider, isFirstGame
	return r.answerWith
}

// TestDealOpeningHandsAfterLetsTheLoserDecide pins determineFirstTurnPlayer's
// non-first-game branch (GameAction.java:2421-2441): the loser of the last
// game is the one asked, with isFirstGame false, whichever seat the coin would
// have picked, and every seated player still gets an opening hand.
func TestDealOpeningHandsAfterLetsTheLoserDecide(t *testing.T) {
	t.Parallel()

	for _, loserSeat := range []int{0, 1} {
		g := newGame(t, "a", "b")
		loser := g.Players()[loserSeat]
		for _, pid := range g.Players() {
			for range 10 {
				g.NewCard(creatureDefPT(t, "1", "1"), pid, engine.Library)
			}
		}
		c := &startingPlayerRecorder{ScriptedController: engine.NewScriptedController(), answerWith: loser}
		first := engine.DealOpeningHandsAfter(g, c, loser)
		if c.asks != 1 || c.decider != loser || c.isFirstGame {
			t.Errorf("seat %d: asks=%d decider=%v isFirstGame=%v, want one ask of the loser with isFirstGame false",
				loserSeat, c.asks, c.decider, c.isFirstGame)
		}
		if first != loser {
			t.Errorf("seat %d: first = %v, want the loser's own answer %v", loserSeat, first, loser)
		}
		for _, pid := range g.Players() {
			if n := len(g.Zone(engine.Hand, pid).Cards()); n != 7 {
				t.Errorf("seat %d: %v has %d cards in hand, want 7", loserSeat, pid, n)
			}
		}
	}
}
