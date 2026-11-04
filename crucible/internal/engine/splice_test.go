package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
	"github.com/jczastkiewicz/crucible/internal/mana"
)

// Splice (CR 702.47): Glacial Ray spliced onto Lava Spike adds its 2 damage
// to the spell, its splice cost to the cost, and stays in the hand; declining
// casts Lava Spike alone.
func TestSpliceAddsTheSplicedCardsText(t *testing.T) {
	t.Parallel()

	for _, splice := range []bool{true, false} {
		g, p, other := newTwoPlayerGameOn(t, scenarioDB(t))
		g.SetTurnState(1, p, engine.Main1)
		spike := g.NewCard(corpusCard(t, "Lava Spike"), p, engine.Hand)
		ray := g.NewCard(corpusCard(t, "Glacial Ray"), p, engine.Hand)
		g.Player(p).ManaPool.Add(mana.Red, 3)

		c := engine.NewScriptedController()
		if splice {
			c.QueueEntityChoice([]engine.EntityID{engine.CardEntity(ray)})
		} else {
			c.QueueEntityChoice(nil)
		}
		c.QueueTargets([]engine.EntityID{engine.PlayerEntity(other)})
		if splice {
			c.QueueTargets([]engine.EntityID{engine.PlayerEntity(other)})
			c.QueuePayGeneric(mana.ShardR)
		}
		castThenResolve(t, g, p, spike, c)
		if err := g.TakePendingError(); err != nil {
			t.Fatalf("splice %v: %v", splice, err)
		}
		want, left := 3, 2
		if splice {
			want, left = 5, 0
		}
		if got := 20 - g.Player(other).Life; got != want {
			t.Errorf("splice %v: opponent lost %d, want %d", splice, got, want)
		}
		if got := g.Player(p).ManaPool.Total(); got != left {
			t.Errorf("splice %v: %d mana left, want %d", splice, got, left)
		}
		if got := g.Card(ray).Zone; got != engine.Hand {
			t.Errorf("splice %v: Glacial Ray in %v, want Hand", splice, got)
		}
	}
}

// Minamo's Meddling counters a spliced spell and its controller discards the
// cards spliced onto it (CounterEffect.java:96-98).
func TestMinamosMeddlingDiscardsTheSplicedCards(t *testing.T) {
	t.Parallel()

	g, p, other := newTwoPlayerGameOn(t, scenarioDB(t))
	g.SetTurnState(1, p, engine.Main1)
	spike := g.NewCard(corpusCard(t, "Lava Spike"), p, engine.Hand)
	ray := g.NewCard(corpusCard(t, "Glacial Ray"), p, engine.Hand)
	g.Player(p).ManaPool.Add(mana.Red, 3)
	c := engine.NewScriptedController()
	c.QueueEntityChoice([]engine.EntityID{engine.CardEntity(ray)})
	c.QueueTargets([]engine.EntityID{engine.PlayerEntity(other)})
	c.QueueTargets([]engine.EntityID{engine.PlayerEntity(other)})
	c.QueuePayGeneric(mana.ShardR)
	if !g.CastSpell(p, spike, c) {
		t.Fatalf("cast Lava Spike: %v", g.TakePendingError())
	}

	meddling := g.NewCard(corpusCard(t, "Minamo's Meddling"), other, engine.Hand)
	g.Player(other).ManaPool.Add(mana.Blue, 4)
	oc := engine.NewScriptedController()
	oc.QueueTargets([]engine.EntityID{engine.CardEntity(spike)})
	oc.QueuePayGeneric(mana.ShardU)
	oc.QueuePayGeneric(mana.ShardU)
	if !g.CastSpell(other, meddling, oc) {
		t.Fatalf("cast Minamo's Meddling: %v", g.TakePendingError())
	}
	if err := g.ResolveStack(engine.NewRegistry(), oc); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if got := g.Player(other).Life; got != 20 {
		t.Errorf("opponent life = %d, want 20 (Lava Spike countered)", got)
	}
	if got := g.Card(ray).Zone; got != engine.Graveyard {
		t.Errorf("Glacial Ray in %v, want Graveyard (discarded)", got)
	}
}
