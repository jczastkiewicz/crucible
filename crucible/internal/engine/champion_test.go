package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
	"github.com/jczastkiewicz/crucible/internal/mana"
)

// Champion (CR 702.72, CardFactoryUtil.java:741-795): Mistbind Clique
// exiles another Faerie its controller controls as it enters, its Championed
// trigger taps the target player's lands, and the Faerie returns when the
// Clique leaves the battlefield.
func TestChampionExilesAndReturnsTheChampionedCard(t *testing.T) {
	t.Parallel()

	g, p, other := newTwoPlayerGameOn(t, scenarioDB(t))
	g.SetTurnState(1, p, engine.Main1)
	faerie := g.NewCard(corpusCard(t, "Scryb Sprites"), p, engine.Battlefield)
	land := g.NewCard(corpusCard(t, "Forest"), other, engine.Battlefield)
	clique := g.NewCard(corpusCard(t, "Mistbind Clique"), p, engine.Hand)
	g.Player(p).ManaPool.Add(mana.Blue, 4)
	c := engine.NewScriptedController()
	for range 3 {
		c.QueuePayGeneric(mana.ShardU)
	}
	c.QueueConfirmEffect(true)
	c.QueueCardChoice([]engine.CardID{faerie})
	c.QueueTargets([]engine.EntityID{engine.PlayerEntity(other)})

	if !g.CastSpell(p, clique, c) {
		t.Fatalf("cast failed: %v", g.TakePendingError())
	}
	if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if got := g.Card(faerie).Zone; got != engine.Exile {
		t.Fatalf("championed Faerie in %v, want Exile", got)
	}
	if !g.Card(land).Tapped {
		t.Error("Championed trigger did not tap the target player's land")
	}
	if got := g.Card(clique).Zone; got != engine.Battlefield {
		t.Fatalf("Mistbind Clique in %v, want Battlefield", got)
	}

	if err := resolveTargeting(t, g, p, c, []engine.EntityID{engine.CardEntity(clique)}, "DB$ Destroy | ValidTgts$ Creature"); err != nil {
		t.Fatalf("destroy: %v", err)
	}
	if got := g.Card(faerie).Zone; got != engine.Battlefield {
		t.Errorf("championed Faerie in %v after the Clique left, want Battlefield", got)
	}
}

// With no other Faerie to champion, Mistbind Clique is sacrificed.
func TestChampionWithNothingToExileSacrificesTheChampion(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGameOn(t, scenarioDB(t))
	g.SetTurnState(1, p, engine.Main1)
	clique := g.NewCard(corpusCard(t, "Mistbind Clique"), p, engine.Hand)
	g.Player(p).ManaPool.Add(mana.Blue, 4)
	c := engine.NewScriptedController()
	for range 3 {
		c.QueuePayGeneric(mana.ShardU)
	}
	c.QueueConfirmEffect(true)
	c.QueueCardChoice(nil)

	if !g.CastSpell(p, clique, c) {
		t.Fatalf("cast failed: %v", g.TakePendingError())
	}
	if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if got := g.Card(clique).Zone; got != engine.Graveyard {
		t.Errorf("Mistbind Clique in %v, want Graveyard", got)
	}
}
