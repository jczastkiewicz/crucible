package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
	"github.com/jczastkiewicz/crucible/internal/mana"
)

// Word of Command: you choose a card from the opponent's hand and they play it
// while you control them; the control ends when that spell has resolved.
func TestWordOfCommandMakesTheOpponentPlayTheChosenCard(t *testing.T) {
	t.Parallel()

	g, p, other := newTwoPlayerGameOn(t, scenarioDB(t))
	g.SetTurnState(1, p, engine.Main1)
	c := engine.NewScriptedController()
	bolt := g.NewCard(corpusCard(t, "Lightning Bolt"), other, engine.Hand)
	word := g.NewCard(corpusCard(t, "Word of Command"), p, engine.Hand)
	g.Player(p).ManaPool.Add(mana.Black, 2)
	g.Player(other).ManaPool.Add(mana.Red, 1)
	c.QueueTargets([]engine.EntityID{engine.PlayerEntity(other)}) // Word's target
	c.QueueCardChoice([]engine.CardID{bolt})
	c.QueueTargets([]engine.EntityID{engine.PlayerEntity(p)}) // the Bolt's
	if !g.CastSpell(p, word, c) {
		t.Fatalf("CastSpell failed: %v", g.TakePendingError())
	}
	if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
		t.Fatalf("ResolveStack: %v (pending %v)", err, g.TakePendingError())
	}
	if z := g.Card(bolt).Zone; z != engine.Graveyard {
		t.Errorf("the Bolt is in %v, want Graveyard (played and resolved)", z)
	}
	if got := g.Player(p).Life; got != 17 {
		t.Errorf("my life = %d, want 17 (the controlled opponent bolted me)", got)
	}
	if g.IsControlled(other) {
		t.Errorf("the opponent is still controlled after the spell resolved")
	}
}
