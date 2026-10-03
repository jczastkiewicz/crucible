package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
)

// CR 704.5z: a player with no speed who controls a permanent with "Start your
// engines!" has speed 1; one who already has speed keeps it.
func TestStartYourEnginesGivesSpeedOne(t *testing.T) {
	t.Parallel()

	g, p, other := newTwoPlayerGameOn(t, scenarioDB(t))
	g.NewCard(corpusCard(t, "Burnout Bashtronaut"), p, engine.Battlefield)
	g.Player(other).Speed = 3
	g.NewCard(corpusCard(t, "Burnout Bashtronaut"), other, engine.Battlefield)
	sba(g)
	if got := g.Player(p).Speed; got != 1 {
		t.Errorf("speed = %d, want 1", got)
	}
	if got := g.Player(other).Speed; got != 3 {
		t.Errorf("a player who already had speed 3 now has %d", got)
	}
}
