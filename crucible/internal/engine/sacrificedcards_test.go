package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
	"github.com/jczastkiewicz/crucible/internal/mana"
)

// Defined$ SacrificedCards is what the cost really sacrificed
// (CostPartWithList.cardList, AbilityUtils.java:309): Sword of the Ages
// exiles the creature it sacrificed from the graveyard.
func TestDefinedSacrificedCardsNamesTheSacrificedCreature(t *testing.T) {
	t.Parallel()
	g, p, _ := newTwoPlayerGame(t)
	fodder := g.NewCard(creatureDefPT(t, "1", "1"), p, engine.Battlefield)
	altar := g.NewCard(creatureDefWithAbility(t, "Test Exiler",
		"AB$ ChangeZone | Cost$ Sac<1/Creature.Other> | Origin$ Graveyard | Destination$ Exile | Defined$ SacrificedCards"),
		p, engine.Battlefield)
	c := engine.NewScriptedController()
	c.QueueSacrificeChoice([]engine.CardID{fodder})
	if !g.ActivateAbility(p, altar, 0, c) {
		t.Fatal("could not activate")
	}
	if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if z := g.Card(fodder).Zone; z != engine.Exile {
		t.Errorf("sacrificed creature is in %v, want Exile", z)
	}
}

// Rescue from the Underworld remembers its target and the creature it
// sacrificed (RememberObjects$ Targeted & SacrificedCards) and returns both
// at the next upkeep.
func TestRescueFromTheUnderworldReturnsTargetAndSacrifice(t *testing.T) {
	t.Parallel()
	g, p, other := newTwoPlayerGameOn(t, scenarioDB(t))
	libraryCards(t, g, p, 6)
	libraryCards(t, g, other, 6)
	dead := g.NewCard(creatureDefPT(t, "2", "2"), p, engine.Graveyard)
	fodder := g.NewCard(creatureDefPT(t, "1", "1"), p, engine.Battlefield)
	rescue := g.NewCard(corpusCard(t, "Rescue from the Underworld"), p, engine.Hand)
	g.Player(p).ManaPool.Add(mana.Black, 1)
	g.Player(p).ManaPool.AddColorless(4)
	c := engine.NewScriptedController()
	for range 4 {
		c.QueuePayGeneric(mana.ShardC)
	}
	c.QueueSacrificeChoice([]engine.CardID{fodder})
	c.QueueTargets([]engine.EntityID{engine.CardEntity(dead)})
	if !g.CastSpell(p, rescue, c) {
		t.Fatal("could not cast Rescue from the Underworld")
	}
	if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	advanceUntil(t, g, c, func() bool { return g.ActivePhase() == engine.Upkeep && g.ActivePlayer() == p && g.Turn() > 1 })
	if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if g.Card(dead).Zone != engine.Battlefield || g.Card(fodder).Zone != engine.Battlefield {
		t.Errorf("dead in %v, fodder in %v; want both on the battlefield", g.Card(dead).Zone, g.Card(fodder).Zone)
	}
}
