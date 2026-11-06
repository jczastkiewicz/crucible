package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
	"github.com/jczastkiewicz/crucible/internal/mana"
)

// Retrace (CR 702.81, GameActionUtil.java:504-508): Raven's Crime is cast from
// the graveyard by discarding a land card on top of its mana cost, and goes
// back to the graveyard.
func TestRetraceCastsFromGraveyardByDiscardingALand(t *testing.T) {
	t.Parallel()
	g, p, other := newTwoPlayerGameOn(t, scenarioDB(t))
	g.SetTurnState(1, p, engine.Main1)
	crime := g.NewCard(corpusCard(t, "Raven's Crime"), p, engine.Graveyard)
	theirs := g.NewCard(corpusCard(t, "Grizzly Bears"), other, engine.Hand)
	g.Player(p).ManaPool.Add(mana.Black, 1)
	c := engine.NewScriptedController()
	if g.CastSpell(p, crime, c) {
		t.Fatal("retraced with no land card in hand")
	}
	forest := g.NewCard(corpusCard(t, "Forest"), p, engine.Hand)
	c.QueueTargets([]engine.EntityID{engine.PlayerEntity(other)})
	c.QueueDiscardChoice([]engine.CardID{forest})
	if !g.CastSpell(p, crime, c) {
		t.Fatalf("retrace failed: %v", g.TakePendingError())
	}
	if got := g.Card(forest).Zone; got != engine.Graveyard {
		t.Errorf("Forest in %v, want discarded", got)
	}
	if got := g.Player(p).ManaPool.Total(); got != 0 {
		t.Errorf("pool %d after paying {B}, want 0", got)
	}
	c.QueueDiscardChoice([]engine.CardID{theirs})
	if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
		t.Fatal(err)
	}
	if got := g.Card(crime).Zone; got != engine.Graveyard {
		t.Errorf("Raven's Crime in %v after resolving, want the graveyard", got)
	}
}

// Jump-start (CR 702.133): Radical Idea is cast from the graveyard by
// discarding any card, and is exiled as it leaves the stack.
func TestJumpStartDiscardsAndExilesTheSpell(t *testing.T) {
	t.Parallel()
	g, p, _ := newTwoPlayerGameOn(t, scenarioDB(t))
	libraryCards(t, g, p, 3)
	g.SetTurnState(1, p, engine.Main1)
	idea := g.NewCard(corpusCard(t, "Radical Idea"), p, engine.Graveyard)
	bears := g.NewCard(corpusCard(t, "Grizzly Bears"), p, engine.Hand)
	g.Player(p).ManaPool.Add(mana.Blue, 2)
	c := engine.NewScriptedController()
	c.QueuePayGeneric(mana.ShardU)
	c.QueueDiscardChoice([]engine.CardID{bears})
	if !g.CastSpell(p, idea, c) {
		t.Fatalf("jump-start failed: %v", g.TakePendingError())
	}
	if got := g.Card(bears).Zone; got != engine.Graveyard {
		t.Errorf("Grizzly Bears in %v, want discarded", got)
	}
	if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
		t.Fatal(err)
	}
	if got := g.Card(idea).Zone; got != engine.Exile {
		t.Errorf("Radical Idea in %v after resolving, want exile", got)
	}
}

// Split second (CR 702.61, MagicStack.isSplitSecondOnStack): with Sudden
// Shock on the stack no spell is cast and no non-mana ability activated.
func TestSplitSecondLocksTheStack(t *testing.T) {
	t.Parallel()
	g, p, other := newTwoPlayerGameOn(t, scenarioDB(t))
	g.SetTurnState(1, p, engine.Main1)
	shock := g.NewCard(corpusCard(t, "Sudden Shock"), p, engine.Hand)
	bolt := g.NewCard(corpusCard(t, "Lightning Bolt"), other, engine.Hand)
	g.Player(p).ManaPool.Add(mana.Red, 2)
	c := engine.NewScriptedController()
	c.QueueTargets([]engine.EntityID{engine.PlayerEntity(other)})
	c.QueuePayGeneric(mana.ShardR)
	if !g.CastSpell(p, shock, c) {
		t.Fatalf("Sudden Shock: %v", g.TakePendingError())
	}
	g.Player(other).ManaPool.Add(mana.Red, 1)
	oc := engine.NewScriptedController()
	oc.QueueTargets([]engine.EntityID{engine.PlayerEntity(p)})
	if g.CastSpell(other, bolt, oc) {
		t.Fatal("Lightning Bolt cast over split second")
	}
	if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
		t.Fatal(err)
	}
	if !g.CastSpell(other, bolt, oc) {
		t.Fatalf("Lightning Bolt after the stack emptied: %v", g.TakePendingError())
	}
}
