package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
	"github.com/jczastkiewicz/crucible/internal/mana"
)

// castAndResolve casts card for p paying generic mana with green, then
// resolves the stack.
func castThenResolve(t *testing.T, g *engine.Game, p engine.PlayerID, card engine.CardID, c *engine.ScriptedController) {
	t.Helper()
	if !g.CastSpell(p, card, c) {
		t.Fatal("cast failed")
	}
	if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
}

// CR 122.6: a permanent that enters "with N counters" has them as it enters, so
// a 0/0 creature survives the next state-based check (K:etbCounter,
// CardFactoryUtil.makeEtbCounter).
func TestEntersWithCounters(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGameOn(t, scenarioDB(t))
	g.SetTurnState(1, p, engine.Main1)
	mouser := g.NewCard(corpusCard(t, "Big Mother Mouser"), p, engine.Hand)
	g.Player(p).ManaPool.Add(mana.Green, 4)
	c := engine.NewScriptedController()
	queueXPayGeneric(c, mana.ShardG, 4)
	castThenResolve(t, g, p, mouser, c)
	engine.CheckStateBasedActions(g, c)
	card := g.Card(mouser)
	if card.Zone != engine.Battlefield {
		t.Fatalf("Big Mother Mouser is in %v, want the battlefield: it entered as 0/0", card.Zone)
	}
	if got := card.Counters.Count(engine.P1P1); got != 2 {
		t.Errorf("+1/+1 counters = %d, want 2", got)
	}
}

// An X in the amount is the X the spell was cast with.
func TestEntersWithXCounters(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGameOn(t, scenarioDB(t))
	g.SetTurnState(1, p, engine.Main1)
	elite := g.NewCard(corpusCard(t, "Broodguard Elite"), p, engine.Hand)
	g.Player(p).ManaPool.Add(mana.Green, 5)
	c := engine.NewScriptedController()
	c.QueuePayX(3)
	queueXPayGeneric(c, mana.ShardG, 5)
	castThenResolve(t, g, p, elite, c)
	if got := g.Card(elite).Counters.Count(engine.P1P1); got != 3 {
		t.Errorf("+1/+1 counters = %d, want 3 for X=3", got)
	}
}

// Another permanent's replacement (Defined$ ReplacedCard) adds counters to what
// enters, on top of its own.
func TestAnotherPermanentAddsCountersAsACreatureEnters(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGameOn(t, scenarioDB(t))
	g.SetTurnState(1, p, engine.Main1)
	g.NewCard(corpusCard(t, "Grumgully, the Generous"), p, engine.Battlefield)
	bears := g.NewCard(corpusCard(t, "Grizzly Bears"), p, engine.Hand)
	g.Player(p).ManaPool.Add(mana.Green, 2)
	c := engine.NewScriptedController()
	queueXPayGeneric(c, mana.ShardG, 1)
	castThenResolve(t, g, p, bears, c)
	if got := g.Card(bears).Counters.Count(engine.P1P1); got != 1 {
		t.Errorf("Grizzly Bears has %d +1/+1 counters, want 1 from Grumgully", got)
	}
}
