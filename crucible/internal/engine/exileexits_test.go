package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
	"github.com/jczastkiewicz/crucible/internal/mana"
)

// A card leaving exile runs ChangesZone lines naming Origin$ Exile
// (GameAction.java:601-613 runs them for every zone change): the watcher's
// trigger fires for an exiled card an effect returns to its owner's hand.
func TestCardLeavingExileFiresExileOriginTriggers(t *testing.T) {
	t.Parallel()
	g, p, _ := newTwoPlayerGame(t)
	g.NewCard(triggerWatcherDef(t, "Test Exile Watcher",
		"Mode$ ChangesZone | Origin$ Exile | Destination$ Any | ValidCard$ Card | TriggerZones$ Battlefield | Execute$ Gain",
		"Gain", "DB$ GainLife | Defined$ You | LifeAmount$ 1"), p, engine.Battlefield)
	exiled := g.NewCard(creatureDefPT(t, "1", "1"), p, engine.Exile)
	if _, err := resolveNow(t, g, p, engine.NewScriptedController(), []engine.EntityID{engine.CardEntity(exiled)},
		"DB$ ChangeZone | ValidTgts$ Card | Origin$ Exile | Destination$ Hand"); err != nil {
		t.Fatal(err)
	}
	if g.Card(exiled).Zone != engine.Hand {
		t.Fatalf("exiled card is in %v, want Hand", g.Card(exiled).Zone)
	}
	if got := g.Player(p).Life; got != 21 {
		t.Errorf("life = %d, want 21: the Origin$ Exile trigger fired once", got)
	}
}

// A spell resolving leaves the stack for the graveyard: Origin$ Stack lines
// (Geth's Summons, Goblin Artisans) fire, a copy of a spell not among them.
func TestSpellLeavingTheStackFiresStackOriginTriggers(t *testing.T) {
	t.Parallel()
	g, p, _ := newTwoPlayerGame(t)
	g.NewCard(triggerWatcherDef(t, "Test Stack Watcher",
		"Mode$ ChangesZone | Origin$ Stack | Destination$ Graveyard | ValidCard$ Card | TriggerZones$ Battlefield | Execute$ Gain",
		"Gain", "DB$ GainLife | Defined$ You | LifeAmount$ 1"), p, engine.Battlefield)
	spell := g.NewCard(gainInstant(t, "Gain", "W", "3"), p, engine.Hand)
	g.Player(p).ManaPool.Add(mana.White, 1)
	c := engine.NewScriptedController()
	if !g.CastSpell(p, spell, c) {
		t.Fatal("could not cast the instant")
	}
	if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
		t.Fatal(err)
	}
	if got := g.Player(p).Life; got != 24 {
		t.Errorf("life = %d, want 24 (3 from the spell, 1 from the Origin$ Stack trigger)", got)
	}
}
