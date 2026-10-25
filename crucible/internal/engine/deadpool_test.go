package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
	"github.com/jczastkiewicz/crucible/internal/mana"
)

// Deadpool, Trading Card: "as he enters, you may exchange his text box and
// another creature's". The chain's ExchangeTextBox acts on the entering card,
// so it runs just after the move, the ChooseCard before it sees the creature.
func TestDeadpoolExchangesTextBoxesAsHeEnters(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGameOn(t, scenarioDB(t))
	c := engine.NewScriptedController()
	g.SetTurnState(1, p, engine.Main1)
	flyer := g.NewCard(scriptDef(t, "Test Flyer", "Creature Bird", "K:Flying"), p, engine.Battlefield)
	dp := g.NewCard(corpusCard(t, "Deadpool, Trading Card"), p, engine.Hand)
	g.Player(p).ManaPool.Add(mana.Red, 3)
	g.Player(p).ManaPool.Add(mana.Black, 1)
	c.QueueConfirmReplacementEffect(true)
	c.QueueCardChoice([]engine.CardID{flyer})
	for range 2 {
		c.QueuePayGeneric(mana.ShardR)
	}
	if !g.CastSpell(p, dp, c) {
		t.Fatalf("CastSpell failed: %v", g.TakePendingError())
	}
	if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
		t.Fatalf("ResolveStack: %v (pending %v)", err, g.TakePendingError())
	}
	if !g.Card(dp).HasKeyword("Flying") {
		t.Errorf("Deadpool has no flying after exchanging with the flyer")
	}
}
