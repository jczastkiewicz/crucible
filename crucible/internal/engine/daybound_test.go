package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
)

// Daybound and Nightbound (CardFactoryUtil.java:879-895, :1522-1533): with
// neither day nor night a daybound permanent makes it day; when it becomes
// night the daybound front face transforms, and when it becomes day again the
// nightbound back face transforms back.
func TestDayboundTransformsWithDayAndNight(t *testing.T) {
	t.Parallel()
	g, p, _ := newTwoPlayerGameOn(t, scenarioDB(t))
	admirer := g.NewCard(corpusCard(t, "Bird Admirer"), p, engine.Battlefield)
	c := engine.NewScriptedController()
	engine.CheckStateBasedActions(g, c)
	if g.DayTime() != engine.Day {
		t.Fatalf("daytime = %v, want Day: a daybound permanent makes it day", g.DayTime())
	}
	if g.Card(admirer).InTransformedState() {
		t.Fatal("Bird Admirer transformed by day")
	}

	g.SetTurnState(1, p, engine.Cleanup)
	for _, pid := range g.Players() {
		g.Player(pid).SpellsCastThisTurn = 0
	}
	g.AdvancePhase(c)
	engine.CheckStateBasedActions(g, c)
	if g.DayTime() != engine.Night {
		t.Fatalf("daytime = %v, want Night after a turn with no spells", g.DayTime())
	}
	if !g.Card(admirer).InTransformedState() || g.Card(admirer).Name() != "Wing Shredder" {
		t.Fatalf("at night the card is %q (transformed %v), want Wing Shredder", g.Card(admirer).Name(), g.Card(admirer).InTransformedState())
	}

	if _, err := resolveNow(t, g, g.ActivePlayer(), c, nil, "DB$ DayTime | Value$ Day"); err != nil {
		t.Fatal(err)
	}
	engine.CheckStateBasedActions(g, c)
	if g.Card(admirer).InTransformedState() {
		t.Errorf("by day the card is still %q, want Bird Admirer", g.Card(admirer).Name())
	}
}
