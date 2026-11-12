package engine_test

import (
	"slices"
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
	"github.com/jczastkiewicz/crucible/internal/mana"
)

// An "as it enters" replacement whose ReplaceWith$ is a RepeatEach runs before
// the move like the other entry choices (CardFactoryUtil.java:515-543):
// Banescale Serpent notes, for its one opponent, the creature type most
// common in that player's library, and its static then gives it protection
// from that type, so a Goblin cannot block it.
func TestBanescaleSerpentNotesTheOpponentsMostPrevalentType(t *testing.T) {
	t.Parallel()

	g, p, other := newTwoPlayerGameOn(t, scenarioDB(t))
	for range 2 {
		g.NewCard(corpusCard(t, "Goblin Piker"), other, engine.Library)
	}
	g.NewCard(corpusCard(t, "Grizzly Bears"), other, engine.Library)
	goblin := g.NewCard(corpusCard(t, "Goblin Piker"), other, engine.Battlefield)
	bears := g.NewCard(corpusCard(t, "Grizzly Bears"), other, engine.Battlefield)
	serpent := g.NewCard(corpusCard(t, "Banescale Serpent"), p, engine.Hand)
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).ManaPool.Add(mana.Blue, 4)
	g.Player(p).ManaPool.Add(mana.Red, 1)
	c := engine.NewScriptedController()
	for range 3 {
		c.QueuePayGeneric(mana.ShardU)
	}
	c.QueueOption(0)
	if !g.CastSpell(p, serpent, c) {
		t.Fatalf("CastSpell failed: %v", g.TakePendingError())
	}
	if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if err := g.TakePendingError(); err != nil {
		t.Fatalf("pending: %v", err)
	}
	if got := g.Card(serpent).Memory.NotedTypes(); !slices.Equal(got, []string{"Goblin"}) {
		t.Fatalf("noted = %v, want [Goblin]", got)
	}
	if !g.CanBlock(serpent, bears) {
		t.Errorf("Grizzly Bears cannot block Banescale Serpent, want it can")
	}
	if g.CanBlock(serpent, goblin) {
		t.Errorf("Goblin Piker can block Banescale Serpent, want protection from Goblins")
	}
}
