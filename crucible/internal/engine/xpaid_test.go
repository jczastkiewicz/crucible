package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
	"github.com/jczastkiewicz/crucible/internal/mana"
)

// Count$xPaid (AbilityUtils.java:1631): an ability reads the X its caster
// announced. Blaze deals X damage to any target.
func TestXPaidIsTheAnnouncedX(t *testing.T) {
	t.Parallel()

	g, p, other := newTwoPlayerGameOn(t, scenarioDB(t))
	g.SetTurnState(1, p, engine.Main1)
	g.Player(other).Life = 20
	blaze := g.NewCard(corpusCard(t, "Blaze"), p, engine.Hand)
	g.Player(p).ManaPool.Add(mana.Red, 5)
	c := engine.NewScriptedController()
	c.QueuePayX(4)
	queueXPayGeneric(c, mana.ShardR, 4)
	c.QueueTargets([]engine.EntityID{engine.PlayerEntity(other)})
	if !g.CastSpell(p, blaze, c) {
		t.Fatal("cast failed")
	}
	if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if got := g.Player(other).Life; got != 16 {
		t.Errorf("life = %d, want 16 after X=4 damage", got)
	}
}
