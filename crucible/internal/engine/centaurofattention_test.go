package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
	"github.com/jczastkiewicz/crucible/internal/mana"
)

// Centaur of Attention: five dice stored on it when it enters, +X/+X for the
// most of one face among them, a may-reroll per stored result at the
// beginning of combat. Seeds differ, so the checks are the bounds X must keep
// (1 to 5) and that declining every reroll changes nothing.
func TestCentaurOfAttentionStoresFiveDiceAndGrowsByTheLargestSet(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGameOn(t, scenarioDB(t))
	c := engine.NewScriptedController()
	g.SetTurnState(1, p, engine.Main1)
	centaur := g.NewCard(corpusCard(t, "Centaur of Attention"), p, engine.Hand)
	g.Player(p).ManaPool.Add(mana.Green, 5)
	for range 3 {
		c.QueuePayGeneric(mana.ShardG)
	}
	if !g.CastSpell(p, centaur, c) {
		t.Fatalf("CastSpell failed: %v", g.TakePendingError())
	}
	if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
		t.Fatalf("ResolveStack: %v (pending %v)", err, g.TakePendingError())
	}
	power := func() int { pw, _ := g.Card(centaur).Power(); return pw }
	before := power()
	if before < 4 || before > 8 {
		t.Fatalf("power = %d, want 3 + X with X in 1..5", before)
	}
	for range 5 {
		c.QueueConfirmEffect(false)
	}
	toStep(t, g, c, p, engine.CombatBegin, nil)
	if got := power(); got != before {
		t.Errorf("power after declining every reroll = %d, want %d", got, before)
	}
}
