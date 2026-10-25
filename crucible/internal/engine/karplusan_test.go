package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
)

// Karplusan Minotaur: "Cumulative upkeep -- Flip a coin." The flip is the
// cost (CostFlipCoin), always payable, and its win or loss fires the card's
// own coin-flip triggers, each dealing 1 damage.
func TestKarplusanMinotaurPaysItsUpkeepByFlipping(t *testing.T) {
	t.Parallel()

	g, p, other := newTwoPlayerGameOn(t, scenarioDB(t))
	libraryCards(t, g, p, 8)
	libraryCards(t, g, other, 8)
	minotaur := g.NewCard(corpusCard(t, "Karplusan Minotaur"), p, engine.Battlefield)
	g.SetTurnState(2, p, engine.Untap)
	c := engine.NewScriptedController()
	c.QueueConfirmPayCost(true)
	c.QueueCoinCall(true)
	c.QueueTargets([]engine.EntityID{engine.PlayerEntity(other)})
	c.QueueTargets([]engine.EntityID{engine.PlayerEntity(other)})
	toStep(t, g, c, p, engine.Upkeep, nil)
	if got := g.Card(minotaur).Zone; got != engine.Battlefield {
		t.Fatalf("Minotaur is in %v, want Battlefield", got)
	}
	if got := g.Card(minotaur).Counters.Count(engine.Age); got != 1 {
		t.Errorf("age counters = %d, want 1", got)
	}
	if got := g.Player(other).Life; got != 19 {
		t.Errorf("opponent life = %d, want 19 (one flip, one damage)", got)
	}
}
