package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
	"github.com/jczastkiewicz/crucible/internal/mana"
)

// Nylea's Oracle compares the library, after it conjures X copies of itself
// into it, with StartingLibrarySize, the registered deck's main count
// (Player.java:447-449; ConditionSVarCompare$ GTStartingSize reads the SVar
// by name, SpellAbilityCondition.java:492-507). Devotion to green is 3 (the
// Oracle's GG and Grizzly Bears' G), so it conjures three: a library of 5
// from a deck of 5 grows past it and the creatures get +3/+3; from a deck of
// 40 it does not.
func TestNyleasOracleComparesTheLibraryWithTheStartingDeck(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		deck     int
		wantPump bool
	}{{5, true}, {40, false}} {
		g, p, other := newTwoPlayerGameOn(t, scenarioDB(t))
		g.Player(p).StartingLibrarySize = tc.deck
		libraryCards(t, g, p, 5)
		libraryCards(t, g, other, 5)
		bears := g.NewCard(corpusCard(t, "Grizzly Bears"), p, engine.Battlefield)
		oracle := g.NewCard(corpusCard(t, "Nylea's Oracle"), p, engine.Hand)
		g.SetTurnState(1, p, engine.Main1)
		g.Player(p).ManaPool.Add(mana.Green, 2)
		c := engine.NewScriptedController()
		if !g.CastSpell(p, oracle, c) {
			t.Fatalf("CastSpell failed: %v", g.TakePendingError())
		}
		if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
			t.Fatalf("deck %d: ResolveStack: %v", tc.deck, err)
		}
		if err := g.TakePendingError(); err != nil {
			t.Fatalf("deck %d: pending: %v", tc.deck, err)
		}
		if n := len(g.Zone(engine.Library, p).Cards()); n != 8 {
			t.Errorf("deck %d: library = %d cards, want 8", tc.deck, n)
		}
		power, _ := g.Card(bears).Power()
		want := 2
		if tc.wantPump {
			want = 5
		}
		if power != want {
			t.Errorf("deck %d: Grizzly Bears power = %d, want %d", tc.deck, power, want)
		}
	}
}
