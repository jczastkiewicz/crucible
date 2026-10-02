package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
)

// Mode$ LifeLost | FirstTime$ True (Vengeful Warchief): the first life lost
// each turn puts a +1/+1 counter on it; the second does not.
func TestLifeLostTriggersOnTheFirstLossOfTheTurnOnly(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGameOn(t, scenarioDB(t))
	g.SetTurnState(1, p, engine.Main1)
	chief := g.NewCard(corpusCard(t, "Vengeful Warchief"), p, engine.Battlefield)
	sba(g)
	c := engine.NewScriptedController()
	for _, line := range []string{"DB$ LoseLife | Defined$ You | LifeAmount$ 2", "DB$ LoseLife | Defined$ You | LifeAmount$ 3"} {
		if err := resolveWith(t, g, p, c, line); err != nil {
			t.Fatalf("resolve %q: %v", line, err)
		}
	}
	if got := g.Card(chief).Counters.Count(engine.P1P1); got != 1 {
		t.Errorf("Vengeful Warchief has %d +1/+1 counters, want 1", got)
	}
	if got := g.Player(p).Life; got != 15 {
		t.Errorf("life = %d, want 15", got)
	}
}
