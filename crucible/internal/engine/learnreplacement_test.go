package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
)

// Retriever Phoenix (Event$ Learn, ActiveZones$ Graveyard, Optional$): from the
// graveyard it may return to the battlefield instead of learning, so no card is
// discarded or drawn (Player.learnLesson, Player.java:3940-3943); declining
// learns as usual, and the replacement is silent while the Phoenix is elsewhere.
func TestLearnReplacedByRetrieverPhoenixInTheGraveyard(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name    string
		accept  bool
		zone    engine.ZoneType
		wantNow engine.ZoneType
	}{
		{"accepted", true, engine.Graveyard, engine.Battlefield},
		{"declined", false, engine.Graveyard, engine.Graveyard},
		{"not in the graveyard", true, engine.Hand, engine.Hand},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			g, p, _ := newTwoPlayerGameOn(t, scenarioDB(t))
			g.SetTurnState(1, p, engine.Main1)
			phoenix := g.NewCard(corpusCard(t, "Retriever Phoenix"), p, tc.zone)
			c := engine.NewScriptedController()
			if tc.zone == engine.Graveyard {
				c.QueueConfirmReplacementEffect(tc.accept)
			}
			c.QueueCardChoice(nil)
			resolveLine(t, g, p, c, "DB$ Learn")
			if z := g.Card(phoenix).Zone; z != tc.wantNow {
				t.Errorf("Phoenix is in %v, want %v", z, tc.wantNow)
			}
		})
	}
}
