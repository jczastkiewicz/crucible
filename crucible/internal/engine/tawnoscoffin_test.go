package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
	"github.com/jczastkiewicz/crucible/internal/mana"
)

// Tawnos's Coffin: the exiled creature's counters are noted on the effect, and
// it returns tapped with them when the Coffin leaves the battlefield.
func TestTawnossCoffinReturnsTheCreatureWithItsNotedCounters(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGameOn(t, scenarioDB(t))
	g.SetTurnState(1, p, engine.Main1)
	c := engine.NewScriptedController()
	coffin := g.NewCard(corpusCard(t, "Tawnos's Coffin"), p, engine.Battlefield)
	bears := g.NewCard(corpusCard(t, "Grizzly Bears"), p, engine.Battlefield)
	g.Card(bears).Counters.Add(engine.P1P1, 2)
	g.Player(p).ManaPool.Add(mana.Red, 3)
	for range 3 {
		c.QueuePayGeneric(mana.ShardR)
	}
	c.QueueTargets([]engine.EntityID{engine.CardEntity(bears)})
	if !g.ActivateAbility(p, coffin, 0, c) {
		t.Fatalf("ActivateAbility failed: %v", g.TakePendingError())
	}
	if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
		t.Fatalf("ResolveStack: %v (pending %v)", err, g.TakePendingError())
	}
	if z := g.Card(bears).Zone; z != engine.Exile {
		t.Fatalf("the bears are in %v, want Exile", z)
	}
	c.QueueTargets([]engine.EntityID{engine.CardEntity(coffin)})
	resolveLine(t, g, p, c, "DB$ Destroy | ValidTgts$ Artifact")
	if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
		t.Fatalf("ResolveStack after the Coffin left: %v (pending %v)", err, g.TakePendingError())
	}
	if z := g.Card(bears).Zone; z != engine.Battlefield {
		t.Fatalf("the bears are in %v, want Battlefield", z)
	}
	if got := g.Card(bears).Counters.Count(engine.P1P1); got != 2 {
		t.Errorf("+1/+1 counters = %d, want the 2 noted", got)
	}
	if !g.Card(bears).Tapped {
		t.Errorf("the creature returned untapped, want tapped")
	}
}
