package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
	"github.com/jczastkiewicz/crucible/internal/mana"
)

// Evoke (CR 702.74): Mulldrifter cast for its evoke cost {2}{U} draws two
// cards as it enters and is sacrificed by its evoke trigger; cast for its mana
// cost it draws and stays.
func TestEvokedCreatureIsSacrificedAsItEnters(t *testing.T) {
	t.Parallel()
	for _, evoke := range []bool{true, false} {
		g, p, other := newTwoPlayerGameOn(t, scenarioDB(t))
		libraryCards(t, g, p, 4)
		libraryCards(t, g, other, 4)
		g.SetTurnState(1, p, engine.Main1)
		drifter := g.NewCard(corpusCard(t, "Mulldrifter"), p, engine.Hand)
		c := engine.NewScriptedController()
		g.Player(p).ManaPool.Add(mana.Blue, 5)
		generic := 4
		if evoke {
			c.QueueOption(1)
			generic = 2
		} else {
			c.QueueOption(0)
		}
		for range generic {
			c.QueuePayGeneric(mana.ShardU)
		}
		castThenResolve(t, g, p, drifter, c)
		engine.CheckStateBasedActions(g, c)
		want := engine.Battlefield
		if evoke {
			want = engine.Graveyard
		}
		if got := g.Card(drifter).Zone; got != want {
			t.Errorf("evoke=%v: Mulldrifter is in %v, want %v", evoke, got, want)
		}
		if got := len(g.Zone(engine.Hand, p).Cards()); got != 2 {
			t.Errorf("evoke=%v: hand %d, want the 2 cards Mulldrifter drew", evoke, got)
		}
		if left := 5 - 1 - generic; g.Player(p).ManaPool.Total() != left {
			t.Errorf("evoke=%v: %d mana left, want %d", evoke, g.Player(p).ManaPool.Total(), left)
		}
	}
}

// Solitude's evoke cost is no mana: exiling another white card from the hand
// (ExileFromHand<1/Card.White+Other/white card>), Solitude itself excluded.
func TestEvokeCostCanExileACardFromTheHand(t *testing.T) {
	t.Parallel()
	g, p, other := newTwoPlayerGameOn(t, scenarioDB(t))
	libraryCards(t, g, p, 4)
	libraryCards(t, g, other, 4)
	g.SetTurnState(1, p, engine.Main1)
	solitude := g.NewCard(corpusCard(t, "Solitude"), p, engine.Hand)
	c := engine.NewScriptedController()
	c.QueueOption(1)
	pilgrim := g.NewCard(corpusCard(t, "Squadron Hawk"), p, engine.Hand)
	c.QueueCardChoice([]engine.CardID{pilgrim})
	if !g.CastSpell(p, solitude, c) {
		t.Fatal("Solitude not cast for its evoke cost")
	}
	if got := g.Card(pilgrim).Zone; got != engine.Exile {
		t.Errorf("the white card paid for the evoke is in %v, want exile", got)
	}
	if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	engine.CheckStateBasedActions(g, c)
	if got := g.Card(solitude).Zone; got != engine.Graveyard {
		t.Errorf("the evoked Solitude is in %v, want the graveyard", got)
	}
}
