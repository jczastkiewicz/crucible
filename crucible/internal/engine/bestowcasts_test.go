package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/cardtype"
	"github.com/jczastkiewicz/crucible/internal/engine"
	"github.com/jczastkiewicz/crucible/internal/mana"
)

// castBestowedDryad casts Leafcrown Dryad for its bestow cost {3}{G} on p's
// Grizzly Bears, leaving the spell on the stack.
func castBestowedDryad(t *testing.T) (*engine.Game, engine.PlayerID, engine.CardID, engine.CardID, *engine.ScriptedController) {
	t.Helper()
	g, p, other := newTwoPlayerGameOn(t, scenarioDB(t))
	libraryCards(t, g, p, 4)
	libraryCards(t, g, other, 4)
	g.SetTurnState(1, p, engine.Main1)
	bears := g.NewCard(corpusCard(t, "Grizzly Bears"), p, engine.Battlefield)
	dryad := g.NewCard(corpusCard(t, "Leafcrown Dryad"), p, engine.Hand)
	g.Player(p).ManaPool.Add(mana.Green, 4)
	c := engine.NewScriptedController()
	c.QueueOption(1)
	for range 3 {
		c.QueuePayGeneric(mana.ShardG)
	}
	if !g.CastSpell(p, dryad, c) {
		t.Fatalf("Leafcrown Dryad not cast for its bestow cost: %v", g.TakePendingError())
	}
	if typ := g.Card(dryad).Type(); typ.Has(cardtype.Creature) || !typ.HasSubtype("Aura") {
		t.Fatalf("the bestow spell's type %v, want an Aura and no creature", typ)
	}
	return g, p, bears, dryad, c
}

// Bestow (CR 702.103): cast for its bestow cost, Leafcrown Dryad is an Aura
// spell, not a creature spell, and enters attached to its target, which gets
// +2/+2.
func TestBestowedCreatureEntersAsAnAura(t *testing.T) {
	t.Parallel()
	g, _, bears, dryad, c := castBestowedDryad(t)
	if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
		t.Fatal(err)
	}
	if host, ok := g.Card(dryad).AttachedTo(); !ok || host != bears {
		t.Fatalf("Leafcrown Dryad attached to %v (%v), want Grizzly Bears", host, ok)
	}
	if g.Card(dryad).Type().Has(cardtype.Creature) {
		t.Error("the bestowed Leafcrown Dryad is a creature on the battlefield")
	}
	wantCDAPT(t, g, bears, 4, 4, "Grizzly Bears enchanted by Leafcrown Dryad")
	g.Move(dryad, engine.Graveyard, g.Card(dryad).Owner)
	if g.Card(dryad).Bestowed() || !g.Card(dryad).Type().Has(cardtype.Creature) {
		t.Error("the bestowed Aura put into the graveyard is not the creature card there")
	}
}

// CR 702.103f: when the enchanted creature leaves, the bestowed Aura is not
// put into the graveyard; it becomes a 2/2 creature.
func TestBestowedAuraBecomesACreatureWhenUnattached(t *testing.T) {
	t.Parallel()
	g, p, bears, dryad, c := castBestowedDryad(t)
	if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
		t.Fatal(err)
	}
	if host, ok := g.Card(dryad).AttachedTo(); !ok || host != bears {
		t.Fatalf("Leafcrown Dryad attached to %v (%v), want Grizzly Bears", host, ok)
	}
	g.Move(bears, engine.Graveyard, p)
	engine.CheckStateBasedActions(g, c)
	if got := g.Card(dryad).Zone; got != engine.Battlefield {
		t.Fatalf("Leafcrown Dryad in %v, want the battlefield", got)
	}
	if typ := g.Card(dryad).Type(); !typ.Has(cardtype.Creature) || typ.HasSubtype("Aura") {
		t.Errorf("the unattached Leafcrown Dryad's type %v, want a creature and no Aura", typ)
	}
	wantCDAPT(t, g, dryad, 2, 2, "Leafcrown Dryad as a creature")
}

// CR 702.103e: a bestow spell whose target is gone is not countered; it
// resolves as the creature spell.
func TestBestowSpellWithAGoneTargetResolvesAsACreature(t *testing.T) {
	t.Parallel()
	g, p, bears, dryad, c := castBestowedDryad(t)
	g.Move(bears, engine.Graveyard, p)
	if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
		t.Fatal(err)
	}
	if got := g.Card(dryad).Zone; got != engine.Battlefield {
		t.Fatalf("Leafcrown Dryad in %v, want the battlefield as a creature", got)
	}
	if !g.Card(dryad).Type().Has(cardtype.Creature) {
		t.Error("Leafcrown Dryad entered without being a creature")
	}
}

// A bestow spell that leaves the stack for another zone is the creature card
// there (CounterEffect.java:261), no longer bestowed.
func TestCounteredBestowSpellIsACreatureCard(t *testing.T) {
	t.Parallel()
	g, p, _, dryad, c := castBestowedDryad(t)
	g.Move(dryad, engine.Graveyard, p)
	engine.CheckStateBasedActions(g, c)
	if typ := g.Card(dryad).Type(); !typ.Has(cardtype.Creature) || typ.HasSubtype("Aura") {
		t.Errorf("Leafcrown Dryad in the graveyard has type %v, want the creature card", typ)
	}
	if g.Card(dryad).Bestowed() {
		t.Error("Leafcrown Dryad in the graveyard is still bestowed")
	}
}

// The bestow spell is an Aura spell for every check of its cast: under Steel
// Golem ("You can't cast creature spells") Leafcrown Dryad is not cast as a
// creature but is cast for its bestow cost.
func TestBestowCastIsNotACreatureSpell(t *testing.T) {
	t.Parallel()
	g, p, _ := newTwoPlayerGameOn(t, scenarioDB(t))
	g.SetTurnState(1, p, engine.Main1)
	g.NewCard(corpusCard(t, "Steel Golem"), p, engine.Battlefield)
	bears := g.NewCard(corpusCard(t, "Grizzly Bears"), p, engine.Battlefield)
	dryad := g.NewCard(corpusCard(t, "Leafcrown Dryad"), p, engine.Hand)
	g.Player(p).ManaPool.Add(mana.Green, 4)
	c := engine.NewScriptedController()
	c.QueueOption(0)
	if g.CastSpell(p, dryad, c) {
		t.Fatal("Leafcrown Dryad cast as a creature spell under Steel Golem")
	}
	_ = g.TakePendingError()
	c.QueueOption(1)
	c.QueueEnchantTarget(bears)
	for range 3 {
		c.QueuePayGeneric(mana.ShardG)
	}
	if !g.CastSpell(p, dryad, c) {
		t.Fatalf("Leafcrown Dryad not bestowed under Steel Golem: %v", g.TakePendingError())
	}
}
