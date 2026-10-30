package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
	"github.com/jczastkiewicz/crucible/internal/mana"
)

// Mode$ CountersRemain (Skullbriar, the Walking Grave, GameAction.java:254):
// its counters stay on it as it moves to the graveyard, but not to a hand. A
// card without the static loses them either way.
func TestCountersRemainOnTheWayToTheGraveyard(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		card string
		to   engine.ZoneType
		want int
	}{
		{"Skullbriar, the Walking Grave", engine.Graveyard, 2},
		{"Skullbriar, the Walking Grave", engine.Hand, 0},
		{"Grizzly Bears", engine.Graveyard, 0},
	} {
		g, p, _ := newTwoPlayerGameOn(t, scenarioDB(t))
		id := g.NewCard(corpusCard(t, tc.card), p, engine.Battlefield)
		g.Card(id).Counters.Add(engine.P1P1, 2)
		g.Move(id, tc.to, p)
		if got := g.Card(id).Counters.Count(engine.P1P1); got != tc.want {
			t.Errorf("%s to %v: +1/+1 counters %d, want %d", tc.card, tc.to, got, tc.want)
		}
	}
}

// Mode$ NumLoyaltyAct (SpellAbilityRestriction.java:450-460): Oath of
// Teferi's Twice$ lets a planeswalker activate loyalty abilities twice a
// turn, not three times; without it the second is refused (CR 606.3).
func TestNumLoyaltyActTwicePerTurn(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		oath bool
		want int
	}{{false, 1}, {true, 2}} {
		g, p, _ := newTwoPlayerGameOn(t, scenarioDB(t))
		g.SetTurnState(1, p, engine.Main1)
		if tc.oath {
			g.NewCard(corpusCard(t, "Oath of Teferi"), p, engine.Battlefield)
		}
		ajani := g.NewCard(corpusCard(t, "Ajani Goldmane"), p, engine.Battlefield)
		c := engine.NewScriptedController()
		activated := 0
		for range 3 {
			if !g.ActivateAbility(p, ajani, 0, c) {
				continue
			}
			activated++
			if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
				t.Fatal(err)
			}
		}
		if activated != tc.want {
			t.Errorf("Oath %v: %d activations, want %d", tc.oath, activated, tc.want)
		}
		if got := g.Player(p).Life; got != 20+2*tc.want {
			t.Errorf("Oath %v: life %d, want %d", tc.oath, got, 20+2*tc.want)
		}
	}
}

// Mode$ ManaRestriction (Imperiosaur, SpellAbility.java:447): mana produced by
// basic lands casts it; the same amount of mana from no source does not.
func TestManaRestrictionSpendsOnlyBasicLandMana(t *testing.T) {
	t.Parallel()
	for _, fromLands := range []bool{false, true} {
		g, p, _ := newTwoPlayerGameOn(t, scenarioDB(t))
		g.SetTurnState(1, p, engine.Main1)
		c := engine.NewScriptedController()
		if fromLands {
			for range 4 {
				forest := g.NewCard(corpusCard(t, "Forest"), p, engine.Battlefield)
				if !g.TapLandForMana(p, forest, mana.Green, c) {
					t.Fatalf("Forest did not tap for mana: %v", g.TakePendingError())
				}
			}
		} else {
			g.Player(p).ManaPool.Add(mana.Green, 4)
		}
		dino := g.NewCard(corpusCard(t, "Imperiosaur"), p, engine.Hand)
		for range 2 {
			c.QueuePayGeneric(mana.ShardG)
		}
		if got := g.CastSpell(p, dino, c); got != fromLands {
			t.Errorf("from lands %v: cast %v (%v)", fromLands, got, g.TakePendingError())
		}
	}
}

// Mode$ SetCost | RaiseTo$ (Trinisphere, CostAdjustment.java:433-447): while
// it is untapped a spell that would cost less than three costs three, an X
// spell counting its announced X (CR 601.2f); tapped, it does nothing.
func TestSetCostRaisesSpellsToThree(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name     string
		spell    string
		x        int
		tapped   bool
		wantPaid int
	}{
		{"Shock", "Shock", -1, false, 3},
		{"Shock tapped Trinisphere", "Shock", -1, true, 1},
		{"Blaze X=1", "Blaze", 1, false, 3},
		{"Blaze X=3", "Blaze", 3, false, 4},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			g, p, other := newTwoPlayerGameOn(t, scenarioDB(t))
			g.SetTurnState(1, p, engine.Main1)
			tri := g.NewCard(corpusCard(t, "Trinisphere"), p, engine.Battlefield)
			g.Card(tri).Tapped = tc.tapped
			g.Player(p).ManaPool.Add(mana.Red, 5)
			spell := g.NewCard(corpusCard(t, tc.spell), p, engine.Hand)
			c := engine.NewScriptedController()
			if tc.x >= 0 {
				c.QueuePayX(tc.x)
			}
			c.QueueTargets([]engine.EntityID{engine.PlayerEntity(other)})
			for range 5 {
				c.QueuePayGeneric(mana.ShardR)
			}
			if !g.CastSpell(p, spell, c) {
				t.Fatalf("cast failed: %v", g.TakePendingError())
			}
			if got := 5 - g.Player(p).ManaPool.Total(); got != tc.wantPaid {
				t.Errorf("paid %d, want %d", got, tc.wantPaid)
			}
		})
	}
}
