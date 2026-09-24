package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
)

// TestExploreEffectLandGoesToHand proves CR 701.44a's land branch: the
// revealed land goes to hand and the explorer gets no counter.
func TestExploreEffectLandGoesToHand(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGame(t)
	land := g.NewCard(landDef(t, "Test Land", "Basic Land Plains"), p, engine.Library)

	c := engine.NewScriptedController()
	host, err := castETBChain(t, g, p, etbChainDef(t, "Test Explore Land", "DB$ Explore"), c)
	if err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if z := g.Card(land).Zone; z != engine.Hand {
		t.Errorf("land zone = %v, want Hand", z)
	}
	if n := g.Card(host).Counters.Count(engine.P1P1); n != 0 {
		t.Errorf("explorer +1/+1 counters = %d, want 0", n)
	}
}

// TestExploreEffectNonlandCounterAndChoice proves the nonland branch: a
// +1/+1 counter either way, and the revealed card goes to the graveyard
// only when the controller says so.
func TestExploreEffectNonlandCounterAndChoice(t *testing.T) {
	t.Parallel()

	for _, toGraveyard := range []bool{true, false} {
		g, p, _ := newTwoPlayerGame(t)
		top := libraryCards(t, g, p, 1)[0]
		c := engine.NewScriptedController()
		c.QueueConfirmEffect(toGraveyard)
		host, err := castETBChain(t, g, p, etbChainDef(t, "Test Explore", "DB$ Explore"), c)
		if err != nil {
			t.Fatalf("ResolveStack: %v", err)
		}
		want := engine.Library
		if toGraveyard {
			want = engine.Graveyard
		}
		if z := g.Card(top).Zone; z != want {
			t.Errorf("toGraveyard=%v: revealed zone = %v, want %v", toGraveyard, z, want)
		}
		if n := g.Card(host).Counters.Count(engine.P1P1); n != 1 {
			t.Errorf("toGraveyard=%v: explorer counters = %d, want 1", toGraveyard, n)
		}
	}
}
