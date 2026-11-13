package engine_test

import (
	"slices"
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
)

// discardBatchWatcher is an enchantment whose Mode$ ChangesZoneAll trigger
// gains 3 life once per batch of cards put into a graveyard from a hand.
const discardBatchWatcher = "T:Mode$ ChangesZoneAll | ValidCards$ Card | Origin$ Hand | Destination$ Graveyard | TriggerZones$ Battlefield | Execute$ TrigGain"

// Moonshadow's "one or more permanent cards are put into your graveyard"
// reads a discard's zone batch (DiscardEffect.java:286-295), "your" being
// its controller's: Ragavan's controller cast the opponent's Moonshadow and
// discarded two cards to Seasoned Pyromancer.
func TestStolenMoonshadowLosesACounterForADiscardBatch(t *testing.T) {
	t.Parallel()

	g, p, other := newTwoPlayerGameOn(t, scenarioDB(t))
	g.SetTurnState(1, p, engine.Main1)
	m := g.NewCard(corpusCard(t, "Moonshadow"), other, engine.Battlefield)
	if err := resolveWith(t, g, p, engine.NewScriptedController(), "DB$ GainControl | Defined$ Valid Card.namedMoonshadow | NewController$ You"); err != nil {
		t.Fatalf("GainControl: %v", err)
	}
	g.Card(m).Counters.Add(engine.M1M1, 6)
	g.NewCard(corpusCard(t, "Marsh Flats"), p, engine.Hand)
	g.NewCard(corpusCard(t, "Island"), p, engine.Hand)
	c := engine.NewScriptedController()
	hand := g.Zone(engine.Hand, p).Cards()
	c.QueueDiscardChoice([]engine.CardID{hand[0], hand[1]})
	c.QueueCardOrder([]engine.CardID{hand[0], hand[1]})
	if err := resolveWith(t, g, p, c, "DB$ Discard | Defined$ You | NumCards$ 2 | Mode$ TgtChoose"); err != nil {
		t.Fatalf("Discard: %v", err)
	}
	if got := g.Card(m).Counters.Count(engine.M1M1); got != 5 {
		t.Errorf("Moonshadow's -1/-1 counters = %d, want 5: one removal for the batch of two", got)
	}
}

// A Discard effect that makes every player discard fills one table, so a
// ChangesZoneAll trigger sees one batch (DiscardEffect.java:286-295).
func TestEveryPlayersDiscardIsOneZoneBatch(t *testing.T) {
	t.Parallel()

	g, p, other := newTwoPlayerGameOn(t, scenarioDB(t))
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life = 20
	g.NewCard(scriptDef(t, "Test Watcher", "Enchantment", discardBatchWatcher, gainThreeSVar), p, engine.Battlefield)
	mine := g.NewCard(corpusCard(t, "Island"), p, engine.Hand)
	theirs := g.NewCard(corpusCard(t, "Island"), other, engine.Hand)
	c := engine.NewScriptedController()
	c.QueueDiscardChoice([]engine.CardID{mine})
	c.QueueDiscardChoice([]engine.CardID{theirs})
	if err := resolveWith(t, g, p, c, "DB$ Discard | Defined$ Player | NumCards$ 1 | Mode$ TgtChoose"); err != nil {
		t.Fatalf("Discard: %v", err)
	}
	if got := g.Player(p).Life; got != 23 {
		t.Errorf("life = %d, want 23: one ChangesZoneAll for both players' discards", got)
	}
}

// A Discard<1/Card> cost fills its own table (CostPartWithList.java:178-183).
func TestDiscardCostIsAZoneBatch(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGameOn(t, scenarioDB(t))
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life = 20
	g.NewCard(scriptDef(t, "Test Watcher", "Enchantment", discardBatchWatcher, gainThreeSVar), p, engine.Battlefield)
	g.NewCard(corpusCard(t, "Island"), p, engine.Hand)
	looter := g.NewCard(creatureDefWithAbility(t, "Test Looter", "AB$ GainLife | Cost$ Discard<1/Card> | Defined$ You | LifeAmount$ 0"), p, engine.Battlefield)
	c := engine.NewScriptedController()
	c.QueueDiscardChoice(g.Zone(engine.Hand, p).Cards())
	if !g.ActivateAbility(p, looter, 0, c) {
		t.Fatal("could not activate the discard ability")
	}
	if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if got := g.Player(p).Life; got != 23 {
		t.Errorf("life = %d, want 23: the discard cost's ChangesZoneAll", got)
	}
}

// Cleanup's discard to hand size fires its table's ChangesZoneAll and then
// DiscardedAll (PhaseHandler.java:287-306).
func TestCleanupDiscardFiresTheZoneBatchAndDiscardedAll(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGameOn(t, scenarioDB(t))
	g.SetTurnState(1, p, engine.EndOfTurn)
	g.Player(p).Life = 20
	g.NewCard(scriptDef(t, "Test Watcher", "Enchantment", discardBatchWatcher, gainThreeSVar,
		"T:Mode$ DiscardedAll | ValidPlayer$ You | TriggerZones$ Battlefield | Execute$ TrigGainFive",
		"SVar:TrigGainFive:DB$ GainLife | Defined$ You | LifeAmount$ 5"), p, engine.Battlefield)
	for range 9 {
		g.NewCard(corpusCard(t, "Island"), p, engine.Hand)
	}
	c := engine.NewScriptedController()
	c.KeepCardOrder()
	c.QueueDiscard(slices.Clone(g.Zone(engine.Hand, p).Cards()[:2]))
	g.AdvancePhase(c)
	if g.ActivePhase() != engine.Cleanup {
		t.Fatalf("phase = %v, want Cleanup", g.ActivePhase())
	}
	if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if got := g.Player(p).Life; got != 28 {
		t.Errorf("life = %d, want 28: one ChangesZoneAll (3) and one DiscardedAll (5)", got)
	}
}
