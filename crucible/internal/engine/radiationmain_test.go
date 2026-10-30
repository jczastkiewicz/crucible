package engine_test

import (
	"strconv"
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
)

// CR 121.13 / Player.createRadiationEffect: a player given rad counters gets a
// "Radiation" effect card whose precombat-main trigger mills that many cards
// and, per nonland card milled, costs 1 life and a rad counter. Under Strong,
// the Brutish Thespian's GainLifeRadiation they gain the life instead. With
// no rad counter left the effect card leaves the Command zone.
func TestRadiationMillsAtPrecombatMain(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name     string
		strong   bool
		rad      int
		wantLife int
		wantRad  int
	}{
		{"loses life", false, 3, 18, 1},
		{"Strong gains life", true, 3, 22, 1},
		{"every card nonland", false, 2, 18, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			g, p, _ := newTwoPlayerGameOn(t, scenarioDB(t))
			c := engine.NewScriptedController()
			// Library top first: two nonland cards around a land when rad
			// is 3, two nonland cards when it is 2.
			if tc.rad == 3 {
				g.NewCard(corpusCard(t, "Grizzly Bears"), p, engine.Library)
				g.NewCard(corpusCard(t, "Forest"), p, engine.Library)
				g.NewCard(corpusCard(t, "Grizzly Bears"), p, engine.Library)
			} else {
				g.NewCard(corpusCard(t, "Grizzly Bears"), p, engine.Library)
				g.NewCard(corpusCard(t, "Grizzly Bears"), p, engine.Library)
			}
			libraryCards(t, g, p, 3)
			if tc.strong {
				g.NewCard(corpusCard(t, "Strong, the Brutish Thespian"), p, engine.Battlefield)
			}
			g.SetTurnState(1, p, engine.Main1)
			if _, err := resolveNow(t, g, p, c, nil, "DB$ Radiation | Defined$ You | Num$ "+strconv.Itoa(tc.rad)); err != nil {
				t.Fatal(err)
			}
			if !hasCardNamed(g, g.Zone(engine.Command, p).Cards(), "Radiation") {
				t.Fatal("no Radiation effect card in the Command zone after rad counters")
			}

			g.SetTurnState(1, p, engine.Draw)
			g.AdvancePhase(c)
			if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
				t.Fatal(err)
			}
			if got := len(g.Zone(engine.Graveyard, p).Cards()); got != tc.rad {
				t.Errorf("graveyard %d, want %d milled", got, tc.rad)
			}
			if got := g.Player(p).Life; got != tc.wantLife {
				t.Errorf("life %d, want %d", got, tc.wantLife)
			}
			if got := g.Player(p).Counters.Count(engine.Radiation); got != tc.wantRad {
				t.Errorf("rad counters %d, want %d", got, tc.wantRad)
			}
			if got := hasCardNamed(g, g.Zone(engine.Command, p).Cards(), "Radiation"); got != (tc.wantRad > 0) {
				t.Errorf("Radiation effect card present %v, want %v", got, tc.wantRad > 0)
			}
		})
	}
}

func hasCardNamed(g *engine.Game, ids []engine.CardID, name string) bool {
	for _, id := range ids {
		if g.Card(id).Name() == name {
			return true
		}
	}
	return false
}
