package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
	"github.com/jczastkiewicz/crucible/internal/mana"
)

// Kathril, Aspect Warper: one keyword counter on a creature you control for
// each listed keyword some creature card in your graveyard has. Only your own
// graveyard counts (SharedRestrictions$ Creature.YouOwn). The +1/+1 counters
// follow Java, not the Oracle text: the remembered amounts (1 and 1) sit in a
// set and collapse to one (card-script-defects.md), so Kathril gets 1, not 2.
func TestKathrilCountsOnlyTheKeywordsOfYourGraveyardCreatures(t *testing.T) {
	t.Parallel()

	g, p, other := newTwoPlayerGameOn(t, scenarioDB(t))
	c := engine.NewScriptedController()
	g.SetTurnState(1, p, engine.Main1)
	bears := g.NewCard(corpusCard(t, "Grizzly Bears"), p, engine.Battlefield)
	g.NewCard(corpusCard(t, "Serra Angel"), p, engine.Graveyard)      // flying, vigilance
	g.NewCard(corpusCard(t, "Giant Spider"), other, engine.Graveyard) // reach, not ours
	kathril := g.NewCard(corpusCard(t, "Kathril, Aspect Warper"), p, engine.Hand)
	g.Player(p).ManaPool.Add(mana.White, 1)
	g.Player(p).ManaPool.Add(mana.Black, 1)
	g.Player(p).ManaPool.Add(mana.Green, 3)
	c.QueuePayGeneric(mana.ShardG)
	c.QueuePayGeneric(mana.ShardG)
	for range 2 {
		c.QueueCardChoice([]engine.CardID{bears})
	}
	if !g.CastSpell(p, kathril, c) {
		t.Fatalf("CastSpell failed: %v", g.TakePendingError())
	}
	if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
		t.Fatalf("ResolveStack: %v (pending %v)", err, g.TakePendingError())
	}
	if got := g.Card(kathril).Counters.Count(engine.P1P1); got != 1 {
		t.Errorf("Kathril +1/+1 counters = %d, want 1 (Java's set dedupes the two amounts)", got)
	}
	if got := g.Card(bears).Counters.Count("REACH"); got != 0 {
		t.Errorf("reach counters = %d, want 0 (the spider is the opponent's)", got)
	}
	if got := g.Card(bears).Counters.Count("VIGILANCE"); got != 1 {
		t.Errorf("vigilance counters = %d, want 1", got)
	}
	if got := g.Card(bears).Counters.Count("FLYING"); got != 1 {
		t.Errorf("flying counters = %d, want 1", got)
	}
}
