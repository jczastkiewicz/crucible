package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/cardtype"
	"github.com/jczastkiewicz/crucible/internal/engine"
	"github.com/jczastkiewicz/crucible/internal/mana"
)

// Impending (CardFactoryUtil.java:1329-1337, :2317-2323, :3085-3105,
// :3937-3939): Overlord of the Balemurk cast for {1}{B} enters with five time
// counters and is no creature while it has one; at its controller's end step
// it loses one. Cast for its mana cost it is a creature with none.
func TestOverlordCastForItsImpendingCostIsNoCreatureUntilTheCountersAreGone(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGameOn(t, scenarioDB(t))
	g.SetTurnState(1, p, engine.Main1)
	overlord := g.NewCard(corpusCard(t, "Overlord of the Balemurk"), p, engine.Hand)
	libraryCards(t, g, p, 6)
	g.Player(p).ManaPool.Add(mana.Black, 2)
	c := engine.NewScriptedController()
	c.QueueOption(1) // the impending cast, after the printed one
	c.QueuePayGeneric(mana.ShardB)
	if !g.CastSpell(p, overlord, c) {
		t.Fatalf("Overlord not cast for its impending cost: %v", g.TakePendingError())
	}
	lib := g.Zone(engine.Library, p).Cards()
	c.QueueCardOrder(lib[:4])
	c.QueueConfirmEffect(false)
	if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	o := g.Card(overlord)
	if got := o.Counters.Count(engine.CounterType("TIME")); got != 5 {
		t.Fatalf("Overlord has %d time counters, want 5", got)
	}
	if o.Type().Has(cardtype.Creature) {
		t.Error("Overlord is a creature with time counters on it")
	}
	toStep(t, g, c, p, engine.EndOfTurn, nil)
	if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if got := g.Card(overlord).Counters.Count(engine.CounterType("TIME")); got != 4 {
		t.Errorf("Overlord has %d time counters after the end step, want 4", got)
	}
}

// Delve's cards are exiled with the spell (CostAdjustment.java:268-273), so
// Murktide Regent's "a +1/+1 counter for each instant and sorcery card exiled
// with it" counts them as it enters.
func TestMurktideRegentCountsTheInstantsItDelved(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGameOn(t, scenarioDB(t))
	g.SetTurnState(1, p, engine.Main1)
	murktide := g.NewCard(corpusCard(t, "Murktide Regent"), p, engine.Hand)
	var bolts []engine.CardID
	for range 5 {
		bolts = append(bolts, g.NewCard(corpusCard(t, "Lightning Bolt"), p, engine.Graveyard))
	}
	g.Player(p).ManaPool.Add(mana.Blue, 2)
	c := engine.NewScriptedController()
	c.QueueCardChoice(bolts)
	if !g.CastSpell(p, murktide, c) {
		t.Fatalf("Murktide Regent not cast: %v", g.TakePendingError())
	}
	if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if got := g.Card(murktide).Counters.Count(engine.CounterType("P1P1")); got != 5 {
		t.Errorf("Murktide Regent has %d +1/+1 counters, want 5", got)
	}
}
