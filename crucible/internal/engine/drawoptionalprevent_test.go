package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
)

// Obstinate Familiar (`Event$ Draw | Optional$ True | Prevent$ True`): the
// drawing player may skip the draw (ReplacementHandler.java:364-395); declined,
// the card is drawn.
func TestObstinateFamiliarMaySkipADraw(t *testing.T) {
	t.Parallel()
	for _, skip := range []bool{true, false} {
		g, p, _ := newTwoPlayerGameOn(t, scenarioDB(t))
		g.NewCard(corpusCard(t, "Obstinate Familiar"), p, engine.Battlefield)
		g.NewCard(corpusCard(t, "Island"), p, engine.Library)
		c := engine.NewScriptedController()
		c.QueueConfirmReplacementEffect(skip)
		g.DrawCards(p, 1, c)
		want := 1
		if skip {
			want = 0
		}
		if got := g.Zone(engine.Hand, p).Len(); got != want {
			t.Errorf("skip %v: %d cards in hand, want %d", skip, got, want)
		}
	}
}
