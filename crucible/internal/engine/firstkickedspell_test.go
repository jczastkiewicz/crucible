package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
	"github.com/jczastkiewicz/crucible/internal/mana"
)

// Vine Gecko's OnlyFirstSpell$ with ValidSpell$ Spell.Kicked
// (CostAdjustment.java:541-557): the turn's first kicked spell costs {1}
// less. An unkicked Burst Lightning before it does not use the reduction up,
// so the first kicked one is paid with four red; the second kicked one needs
// all five.
func TestOnlyFirstSpellCountsOnlyTheSpellsValidSpellNames(t *testing.T) {
	t.Parallel()

	g, p, other := newTwoPlayerGameOn(t, scenarioDB(t))
	g.NewCard(corpusCard(t, "Vine Gecko"), p, engine.Battlefield)
	cast := func(kick bool, reds int) bool {
		t.Helper()
		bolt := g.NewCard(corpusCard(t, "Burst Lightning"), p, engine.Hand)
		g.Player(p).ManaPool.Add(mana.Red, reds)
		c := engine.NewScriptedController()
		c.QueueConfirmPayCost(kick)
		for range 4 {
			c.QueuePayGeneric(mana.ShardR)
		}
		c.QueueTargets([]engine.EntityID{engine.PlayerEntity(other)})
		ok := g.CastSpell(p, bolt, c)
		if ok {
			if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
				t.Fatalf("ResolveStack: %v", err)
			}
		}
		if err := g.TakePendingError(); err != nil {
			t.Fatalf("pending error: %v", err)
		}
		g.Player(p).ManaPool.Empty()
		return ok
	}
	if !cast(false, 1) {
		t.Fatal("unkicked Burst Lightning with one red failed")
	}
	if !cast(true, 4) {
		t.Fatal("first kicked Burst Lightning with four red failed, want Vine Gecko's {1} off")
	}
	if cast(true, 4) {
		t.Error("second kicked Burst Lightning cast with four red, want the reduction used up")
	}
}
