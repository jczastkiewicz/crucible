package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
)

const solemnityLine = "S:Mode$ CantPutCounter | ValidCard$ Permanent"

// A Mode$ CantPutCounter static stops the counters a token enters with
// (TokenEffectBase.java:140-145 puts them through the counter table, whose
// replaceCounterEffect skips a card that cannot receive them).
func TestCantPutCounterStopsTokenEnterCounters(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name   string
		static bool
		want   int
	}{
		{"no static", false, 3},
		{"static", true, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			g, p, _ := newTokenGame(t)
			if tc.static {
				g.NewCard(scriptDef(t, "Test Solemnity", "Enchantment", solemnityLine), p, engine.Battlefield)
			}
			def := etbChainDef(t, "Test Incubate", "DB$ Incubate | Amount$ 3")
			if _, err := castETBChain(t, g, p, def, engine.NewScriptedController()); err != nil {
				t.Fatalf("ResolveStack: %v", err)
			}
			incubators := tokensOn(g, p, "Incubator Token")
			if len(incubators) != 1 {
				t.Fatalf("incubators = %d, want 1 (the token still enters)", len(incubators))
			}
			if n := g.Card(incubators[0]).Counters.Count(engine.P1P1); n != tc.want {
				t.Errorf("incubator +1/+1 counters = %d, want %d", n, tc.want)
			}
		})
	}
}

// MakeCard's WithCounter$ is stopped on the battlefield (entering with them)
// and in any other zone (put on the card there) alike
// (MakeCardEffect.java:179-206).
func TestCantPutCounterStopsMakeCardCounters(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		zone   string
		static bool
		want   int
	}{
		{"Battlefield", false, 2},
		{"Battlefield", true, 0},
		{"Hand", false, 2},
		{"Hand", true, 0},
	} {
		t.Run(tc.zone, func(t *testing.T) {
			t.Parallel()

			bear := namedCreature(t, "Grizzly Bears", "1 G")
			g, p, _ := newPackGame(t, bear)
			if tc.static {
				g.NewCard(scriptDef(t, "Test Solemnity", "Enchantment", solemnityLine), p, engine.Battlefield)
			}
			resolveLine(t, g, p, engine.NewScriptedController(),
				"DB$ MakeCard | Name$ Grizzly Bears | Zone$ "+tc.zone+" | WithCounter$ P1P1 | WithCounterNum$ 2")
			zone := engine.Battlefield
			if tc.zone == "Hand" {
				zone = engine.Hand
			}
			var made []engine.CardID
			for _, id := range g.Zone(zone, p).Cards() {
				if c := g.Card(id); c.Def != nil && c.Def.Name == "Grizzly Bears" {
					made = append(made, id)
				}
			}
			if len(made) != 1 {
				t.Fatalf("conjured bears in %s = %d, want 1", tc.zone, len(made))
			}
			if n := g.Card(made[0]).Counters.Count(engine.P1P1); n != tc.want {
				t.Errorf("counters on the conjured bear = %d, want %d", n, tc.want)
			}
		})
	}
}

// A planeswalker or Battle enters with no loyalty or defense counters under a
// Mode$ CantPutCounter static naming it (Card.canReceiveCounters).
func TestCantPutCounterStopsPrintedLoyaltyAndDefense(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name   string
		static bool
		want   int
	}{
		{"no static", false, 4},
		{"static", true, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			g, p, _ := newTwoPlayerGame(t)
			if tc.static {
				g.NewCard(scriptDef(t, "Test Solemnity", "Enchantment", solemnityLine), p, engine.Battlefield)
			}
			pw := g.NewCard(planeswalkerDefLoyalty(t, "4"), p, engine.Hand)
			g.Move(pw, engine.Battlefield, p)
			if n := g.Card(pw).Counters.Count(engine.Loyalty); n != tc.want {
				t.Errorf("loyalty counters = %d, want %d", n, tc.want)
			}
			battle := g.NewCard(battleDefDefense(t, "5"), p, engine.Hand)
			g.Move(battle, engine.Battlefield, p)
			wantDefense := 5
			if tc.static {
				wantDefense = 0
			}
			if n := g.Card(battle).Counters.Count(engine.Defense); n != wantDefense {
				t.Errorf("defense counters = %d, want %d", n, wantDefense)
			}
		})
	}
}
