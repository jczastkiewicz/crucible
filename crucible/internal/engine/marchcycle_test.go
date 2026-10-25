package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
	"github.com/jczastkiewicz/crucible/internal/mana"
)

// March of Reckless Joy: "As an additional cost you may exile any number of
// red cards from your hand; this spell costs {2} less for each." The count is
// announced (Announce$ Exiled) before X, read as Y by the cost part and as Z by
// the reduction.
func TestMarchOfRecklessJoyExilesRedCardsToReduceItsCost(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGameOn(t, scenarioDB(t))
	c := engine.NewScriptedController()
	g.SetTurnState(1, p, engine.Main1)
	libraryCards(t, g, p, 6)
	red := g.NewCard(corpusCard(t, "Lightning Bolt"), p, engine.Hand)
	g.NewCard(corpusCard(t, "Grizzly Bears"), p, engine.Hand)
	march := g.NewCard(corpusCard(t, "March of Reckless Joy"), p, engine.Hand)
	g.Player(p).ManaPool.Add(mana.Red, 1)
	c.QueueNumberChoice(1)
	c.QueuePayX(2)
	c.QueueCardChoice([]engine.CardID{red})
	if !g.CastSpell(p, march, c) {
		t.Fatalf("CastSpell failed: %v", g.TakePendingError())
	}
	if z := g.Card(red).Zone; z != engine.Exile {
		t.Errorf("the red card is in %v, want Exile (cost)", z)
	}
	if got := g.Player(p).ManaPool.Total(); got != 0 {
		t.Errorf("mana left = %d, want 0: X=2 and {R} paid by {R}, two less for one card", got)
	}
}
