package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/carddb/compile"
	"github.com/jczastkiewicz/crucible/internal/engine"
	"github.com/jczastkiewicz/crucible/internal/mana"
)

// Surge (CR 702.117, CardFactoryUtil.java:3535-3546) and Freerunning
// (CR 702.173, :3038-3065): the keyword's cost is one more way to cast the
// spell, offered while the history gate holds; Condition$ Surge and
// Count$Freerunning read the way.

func nonlandPermanents(g *engine.Game, p engine.PlayerID) int {
	return len(g.Zone(engine.Battlefield, p).Cards())
}

// Crush of Tentacles cast for its surge cost {3}{U}{U} after another spell
// this turn also makes the Octopus; cast for {4}{U}{U} it does not, and with
// no earlier spell the surge cost is not offered.
func TestSurgeCastAddsTheOctopus(t *testing.T) {
	t.Parallel()

	for _, surge := range []bool{true, false} {
		g, p, _ := newTwoPlayerGameOn(t, scenarioDB(t))
		g.SetTurnState(1, p, engine.Main1)
		first := g.NewCard(corpusCard(t, "Ornithopter"), p, engine.Hand)
		crush := g.NewCard(corpusCard(t, "Crush of Tentacles"), p, engine.Hand)
		c := engine.NewScriptedController()
		castThenResolve(t, g, p, first, c)
		before := nonlandPermanents(g, p)
		if surge {
			g.Player(p).ManaPool.Add(mana.Blue, 5)
			c.QueueOption(1)
			for range 3 {
				c.QueuePayGeneric(mana.ShardU)
			}
		} else {
			g.Player(p).ManaPool.Add(mana.Blue, 6)
			c.QueueOption(0)
			for range 4 {
				c.QueuePayGeneric(mana.ShardU)
			}
		}
		castThenResolve(t, g, p, crush, c)
		if err := g.TakePendingError(); err != nil {
			t.Fatalf("surge=%v: pending error: %v", surge, err)
		}
		// Ornithopter returns to hand; the Octopus is all that stays.
		want := 0
		if surge {
			want = 1
		}
		if got := nonlandPermanents(g, p); got != want {
			t.Errorf("surge=%v: %d permanents (had %d before), want %d", surge, got, before, want)
		}
		if got := g.Player(p).ManaPool.Total(); got != 0 {
			t.Errorf("surge=%v: %d mana left, want 0", surge, got)
		}
	}
}

func TestSurgeNeedsAnEarlierSpell(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGameOn(t, scenarioDB(t))
	g.SetTurnState(1, p, engine.Main1)
	crush := g.NewCard(corpusCard(t, "Crush of Tentacles"), p, engine.Hand)
	g.Player(p).ManaPool.Add(mana.Blue, 6)
	c := engine.NewScriptedController() // no option answer queued: only one way
	for range 4 {
		c.QueuePayGeneric(mana.ShardU)
	}
	castThenResolve(t, g, p, crush, c)
	if err := g.TakePendingError(); err != nil {
		t.Fatalf("pending error: %v", err)
	}
	if got := nonlandPermanents(g, p); got != 0 {
		t.Errorf("%d permanents, want 0 (no surge, no Octopus)", got)
	}
}

func assassinDef(t *testing.T) *compile.Card {
	return amountDef(t, "Test Assassin", "Creature Human Assassin", "R", "2", "2", nil)
}

// Monastery Raid exiles two cards; for its freerunning cost {X}{R}, offered
// once an Assassin dealt combat damage to a player this turn, X cards.
func TestFreerunningExilesXCards(t *testing.T) {
	t.Parallel()

	for _, free := range []bool{true, false} {
		g, p, other := newTwoPlayerGameOn(t, scenarioDB(t))
		g.SetTurnState(1, p, engine.Main1)
		libraryCards(t, g, p, 10)
		if free {
			assassin := g.NewCard(assassinDef(t), p, engine.Battlefield)
			ac := engine.NewScriptedController()
			ac.QueueAttackers([]engine.CardID{assassin})
			declareAttackers(t, g, ac)
			bc := engine.NewScriptedController()
			bc.QueueBlocks(nil)
			declareBlockers(t, g, bc)
			g.DealCombatDamage(engine.NewScriptedController())
			if g.Player(other).Life != 18 {
				t.Fatalf("defender life = %d, want 18", g.Player(other).Life)
			}
		}
		raid := g.NewCard(corpusCard(t, "Monastery Raid"), p, engine.Hand)
		c := engine.NewScriptedController()
		if free {
			g.Player(p).ManaPool.Add(mana.Red, 4)
			c.QueueOption(1)
			c.QueuePayX(3)
			for range 3 {
				c.QueuePayGeneric(mana.ShardR)
			}
		} else {
			g.Player(p).ManaPool.Add(mana.Red, 3)
			for range 2 {
				c.QueuePayGeneric(mana.ShardR)
			}
		}
		castThenResolve(t, g, p, raid, c)
		if err := g.TakePendingError(); err != nil {
			t.Fatalf("freerunning=%v: pending error: %v", free, err)
		}
		want := 2
		if free {
			want = 3
		}
		if got := len(g.Zone(engine.Exile, p).Cards()); got != want {
			t.Errorf("freerunning=%v: %d cards exiled, want %d", free, got, want)
		}
	}
}
