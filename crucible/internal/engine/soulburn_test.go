package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
	"github.com/jczastkiewicz/crucible/internal/mana"
)

// Soul Burn: X is paid with black or red mana only, and the life gained is
// capped by the black mana spent on X (Count$xColorPaid B) -- the {B} in the
// rest of the cost does not count.
func TestSoulBurnGainsAtMostTheBlackManaPaidForX(t *testing.T) {
	t.Parallel()

	g, p, other := newTwoPlayerGameOn(t, scenarioDB(t))
	c := engine.NewScriptedController()
	g.SetTurnState(1, p, engine.Main1)
	burn := g.NewCard(corpusCard(t, "Soul Burn"), p, engine.Hand)
	g.Player(p).ManaPool.Add(mana.Black, 4)
	g.Player(p).ManaPool.Add(mana.Red, 1)
	c.QueuePayX(2)
	c.QueueHybridManaColor(mana.Black)
	c.QueueHybridManaColor(mana.Red)
	c.QueuePayGeneric(mana.ShardB)
	c.QueuePayGeneric(mana.ShardB)
	c.QueueTargets([]engine.EntityID{engine.PlayerEntity(other)})
	if !g.CastSpell(p, burn, c) {
		t.Fatalf("CastSpell failed: %v", g.TakePendingError())
	}
	if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
		t.Fatalf("ResolveStack: %v (pending %v)", err, g.TakePendingError())
	}
	if got := g.Player(other).Life; got != 18 {
		t.Errorf("opponent life = %d, want 18", got)
	}
	if got := g.Player(p).Life; got != 21 {
		t.Errorf("caster life = %d, want 21 (one black mana paid X)", got)
	}
}
