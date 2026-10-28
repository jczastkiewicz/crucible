package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
)

// Mode$ LifeLostAll | ValidPlayer$ Opponent | ValidAmountEach$ EQ1 (Ob
// Nixilis, Captive Kingpin): an opponent losing exactly 1 life grows it, a
// loss of 2 does not (TriggerLifeLostAll.filteredMap).
func TestLifeLostAllComparesTheAmountEachOpponentLost(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGameOn(t, scenarioDB(t))
	g.SetTurnState(1, p, engine.Main1)
	ob := g.NewCard(corpusCard(t, "Ob Nixilis, Captive Kingpin"), p, engine.Battlefield)
	sba(g)
	c := engine.NewScriptedController()
	for _, tc := range []struct {
		amount string
		want   int
	}{{"2", 0}, {"1", 1}} {
		line := "DB$ LoseLife | Defined$ Player.Opponent | LifeAmount$ " + tc.amount
		if err := resolveWith(t, g, p, c, line); err != nil {
			t.Fatalf("resolve %q: %v", line, err)
		}
		if got := g.Card(ob).Counters.Count(engine.P1P1); got != tc.want {
			t.Errorf("after losing %s: %d +1/+1 counters, want %d", tc.amount, got, tc.want)
		}
	}
}
