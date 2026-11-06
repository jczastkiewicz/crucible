package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
	"github.com/jczastkiewicz/crucible/internal/mana"
)

// CR 605.1a (SpellAbility.isManaAbility, SpellAbility.java:394-440): a
// mana-producing ability whose sub-ability draws a card is no mana ability.
// Chromatic Sphere is refused as one; activated, it goes on the stack, and
// on resolution adds its mana and draws.
func TestChromaticSphereDrawMakesItAStackAbility(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGameOn(t, scenarioDB(t))
	g.SetTurnState(1, p, engine.Main1)
	g.NewCard(corpusCard(t, "Island"), p, engine.Library)
	sphere := g.NewCard(corpusCard(t, "Chromatic Sphere"), p, engine.Battlefield)
	c := engine.NewScriptedController()
	if g.ActivateManaAbility(p, sphere, 0, c) {
		t.Fatal("ActivateManaAbility took Chromatic Sphere's draw ability, want it refused")
	}
	g.Player(p).ManaPool.Add(mana.Green, 1)
	c.QueuePayGeneric(mana.ShardG)
	c.QueueManaColor(mana.Blue)
	if !g.ActivateAbility(p, sphere, 0, c) {
		t.Fatalf("ActivateAbility failed: %v", g.TakePendingError())
	}
	if got := g.Player(p).ManaPool.Total(); got != 0 {
		t.Errorf("mana in pool before resolution = %d, want 0 (the {1} paid)", got)
	}
	if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if err := g.TakePendingError(); err != nil {
		t.Fatalf("pending error: %v", err)
	}
	if got := g.Player(p).ManaPool.Total(); got != 1 {
		t.Errorf("mana in pool = %d, want 1", got)
	}
	if got := g.Zone(engine.Hand, p).Len(); got != 1 {
		t.Errorf("hand = %d cards, want 1 (the draw)", got)
	}
}

// A Mill cost moves a card from the library (LibraryMovementCostVisitor), so
// Charmed Pendant's ability is refused as a mana ability, as a plain {T}: Add
// {C} is not.
func TestMillCostIsNoManaAbility(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGameOn(t, scenarioDB(t))
	g.SetTurnState(1, p, engine.Main1)
	g.NewCard(corpusCard(t, "Island"), p, engine.Library)
	pendant := g.NewCard(corpusCard(t, "Charmed Pendant"), p, engine.Battlefield)
	wastes := g.NewCard(corpusCard(t, "Wastes"), p, engine.Battlefield)
	c := engine.NewScriptedController()
	if g.ActivateManaAbility(p, pendant, 0, c) {
		t.Error("ActivateManaAbility took Charmed Pendant's Mill ability, want it refused")
	}
	if !g.ActivateManaAbility(p, wastes, 0, c) {
		t.Errorf("ActivateManaAbility refused Wastes: %v", g.TakePendingError())
	}
}

// Colorless asked of TapLandForMana taps the land's {C} ability: Wastes has
// no basic land type that makes it.
func TestTapLandForColorlessUsesTheCAbility(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGameOn(t, scenarioDB(t))
	g.SetTurnState(1, p, engine.Main1)
	wastes := g.NewCard(corpusCard(t, "Wastes"), p, engine.Battlefield)
	island := g.NewCard(corpusCard(t, "Island"), p, engine.Battlefield)
	c := engine.NewScriptedController()
	if g.TapLandForMana(p, island, 0, c) {
		t.Error("TapLandForMana tapped an Island for {C}")
	}
	if !g.TapLandForMana(p, wastes, 0, c) {
		t.Fatalf("TapLandForMana refused Wastes for {C}: %v", g.TakePendingError())
	}
	if !g.Card(wastes).Tapped || g.Player(p).ManaPool.Total() != 1 {
		t.Errorf("tapped = %v, pool = %d; want tapped and 1", g.Card(wastes).Tapped, g.Player(p).ManaPool.Total())
	}
}
