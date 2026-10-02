package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
)

// Mode$ MaxCounter (Rasputin Dreamweaver): at most seven dream counters, so a
// counter put on one already holding seven does nothing and a larger batch is
// cut down to fit (Card.addCounterInternal, Card.java:1760).
func TestMaxCounterCapsDreamCounters(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGameOn(t, scenarioDB(t))
	g.SetTurnState(1, p, engine.Main1)
	rasputin := g.NewCard(corpusCard(t, "Rasputin Dreamweaver"), p, engine.Battlefield)
	bears := g.NewCard(corpusCard(t, "Grizzly Bears"), p, engine.Battlefield)
	sba(g)
	c := engine.NewScriptedController()

	g.Card(rasputin).Counters.Add("DREAM", 5)
	put := func(target engine.CardID, n string) {
		t.Helper()
		line := "DB$ PutCounter | ValidTgts$ Creature | CounterType$ DREAM | CounterNum$ " + n
		if err := resolveTargeting(t, g, p, c, []engine.EntityID{engine.CardEntity(target)}, line); err != nil {
			t.Fatalf("resolve %q: %v", line, err)
		}
	}
	put(rasputin, "4")
	if got := g.Card(rasputin).Counters.Count("DREAM"); got != 7 {
		t.Errorf("Rasputin has %d dream counters after +4 on 5, want 7", got)
	}
	put(rasputin, "1")
	if got := g.Card(rasputin).Counters.Count("DREAM"); got != 7 {
		t.Errorf("Rasputin has %d dream counters after +1 on 7, want 7", got)
	}
	put(bears, "9")
	if got := g.Card(bears).Counters.Count("DREAM"); got != 9 {
		t.Errorf("Grizzly Bears has %d dream counters, want 9 (the cap is Rasputin's own)", got)
	}
}

// CR 704.5r: counters placed past the cap by any path that bypasses
// addCardCounters are trimmed by the next state-based-action pass.
func TestMaxCounterTrimsExcessDreamCountersAsAStateBasedAction(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGameOn(t, scenarioDB(t))
	g.SetTurnState(1, p, engine.Main1)
	rasputin := g.NewCard(corpusCard(t, "Rasputin Dreamweaver"), p, engine.Battlefield)
	g.Card(rasputin).Counters.Add("DREAM", 10)
	sba(g)
	if got := g.Card(rasputin).Counters.Count("DREAM"); got != 7 {
		t.Errorf("Rasputin has %d dream counters after the pass, want 7", got)
	}
}
