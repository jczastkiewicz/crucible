package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
)

// MakeCard FaceDown$ (Darigaaz, Shivan Champion): the conjured card goes to
// Exile and is turned face down there once its counters are on
// (MakeCardEffect.java:246-247).
func TestMakeCardFaceDownConjuresIntoExileFaceDown(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGameOn(t, scenarioDB(t))
	c := engine.NewScriptedController()
	resolveLine(t, g, p, c, "DB$ MakeCard | Conjure$ True | Name$ Grizzly Bears | Zone$ Exile | FaceDown$ True | WithCounter$ P1P1 | WithCounterNum$ 3")
	exiled := g.Zone(engine.Exile, p).Cards()
	if len(exiled) != 1 {
		t.Fatalf("%d cards in exile, want the one conjured", len(exiled))
	}
	card := g.Card(exiled[0])
	if !card.IsFaceDown() {
		t.Error("the conjured card is face up in exile")
	}
	if got := card.Counters.Count(engine.P1P1); got != 3 {
		t.Errorf("+1/+1 counters = %d, want 3", got)
	}
}
