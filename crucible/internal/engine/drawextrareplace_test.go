package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
)

// Reed Richards, Smartest Man (FirstExtraCardDrawnThisTurn$, ReplaceDraw.java:
// 66-69): outside the draw step the first card drawn this turn becomes four;
// the next draw is a plain one.
func TestReedRichardsReplacesTheFirstExtraDraw(t *testing.T) {
	t.Parallel()
	g, p, _ := newTwoPlayerGameOn(t, scenarioDB(t))
	g.NewCard(corpusCard(t, "Reed Richards, Smartest Man"), p, engine.Battlefield)
	for i := 0; i < 10; i++ {
		g.NewCard(corpusCard(t, "Island"), p, engine.Library)
	}
	c := engine.NewScriptedController()
	g.DrawCards(p, 1, c)
	if got := g.Zone(engine.Hand, p).Len(); got != 4 {
		t.Fatalf("after the first draw %d cards in hand, want 4", got)
	}
	g.DrawCards(p, 1, c)
	if got := g.Zone(engine.Hand, p).Len(); got != 5 {
		t.Errorf("after the second draw %d cards in hand, want 5", got)
	}
}
