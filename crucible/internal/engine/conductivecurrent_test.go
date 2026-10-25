package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
	"github.com/jczastkiewicz/crucible/internal/mana"
)

// Conductive Current: the chosen instant or sorcery in hand perpetually gains
// "if this spell would deal damage, it deals that much plus 2 instead" -- read
// from the card in hand once it is cast and deals damage.
func TestConductiveCurrentMakesTheChosenSpellDealTwoMore(t *testing.T) {
	t.Parallel()

	g, p, other := newTwoPlayerGameOn(t, scenarioDB(t))
	g.SetTurnState(1, p, engine.Main1)
	c := engine.NewScriptedController()
	bolt := g.NewCard(corpusCard(t, "Lightning Bolt"), p, engine.Hand)
	current := g.NewCard(corpusCard(t, "Conductive Current"), p, engine.Hand)
	g.Player(p).ManaPool.Add(mana.Red, 5)
	c.QueueCardChoice([]engine.CardID{bolt})
	for range 4 {
		c.QueuePayGeneric(mana.ShardR)
	}
	if !g.CastSpell(p, current, c) {
		t.Fatalf("Conductive Current: %v", g.TakePendingError())
	}
	if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
		t.Fatalf("ResolveStack: %v (pending %v)", err, g.TakePendingError())
	}
	c.QueueTargets([]engine.EntityID{engine.PlayerEntity(other)})
	if !g.CastSpell(p, bolt, c) {
		t.Fatalf("Bolt: %v", g.TakePendingError())
	}
	if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
		t.Fatalf("ResolveStack: %v (pending %v)", err, g.TakePendingError())
	}
	if got := g.Player(other).Life; got != 15 {
		t.Errorf("opponent life = %d, want 15 (3 + 2)", got)
	}
}
