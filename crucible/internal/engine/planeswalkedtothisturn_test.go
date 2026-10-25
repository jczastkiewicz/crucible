package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
)

// PlayerCountPropertyYou$PlaneswalkedToThisTurn <name> (Unyaro's SVar X,
// AbilityUtils.java:3695-3706) is 1 once its player planeswalked to a plane of
// that name this turn and 0 for any other name.
func TestPlaneswalkedToThisTurnReadsThePlanesWalkedTo(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGame(t)
	planarDeck(g, p, plainPlane(t, "Unyaro"), plainPlane(t, "Other"))
	c := engine.NewScriptedController()
	gained := func(name string) int {
		before := g.Player(p).Life
		resolveLine(t, g, p, c, "DB$ GainLife | Defined$ You | LifeAmount$ PlayerCountPropertyYou$PlaneswalkedToThisTurn "+name)
		return g.Player(p).Life - before
	}
	if got := gained("Unyaro"); got != 0 {
		t.Errorf("before planeswalking, count = %d, want 0", got)
	}
	resolveLine(t, g, p, c, "DB$ Planeswalk")
	if got := gained("Unyaro"); got != 1 {
		t.Errorf("after walking to Unyaro, count = %d, want 1", got)
	}
	if got := gained("Other"); got != 0 {
		t.Errorf("a plane not walked to counts %d, want 0", got)
	}
}
