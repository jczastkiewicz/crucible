package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
)

// Killing Wave's RepeatEach carries LoseLifeMap$, which only batches the life
// paid into one LifeLostAll event after the loop; the loop itself runs as any
// other player loop does.
func TestRepeatEachWithLoseLifeMapStillLoopsOverPlayers(t *testing.T) {
	t.Parallel()

	g, p, other := newTwoPlayerGame(t)
	resolveLine(t, g, p, engine.NewScriptedController(),
		"DB$ RepeatEach | RepeatPlayers$ Player | LoseLifeMap$ True | RepeatSubAbility$ Gain",
		"Gain", "DB$ GainLife | Defined$ Remembered | LifeAmount$ 3")
	if g.Player(p).Life != 23 || g.Player(other).Life != 23 {
		t.Errorf("life = %d and %d, want 23 and 23", g.Player(p).Life, g.Player(other).Life)
	}
}
