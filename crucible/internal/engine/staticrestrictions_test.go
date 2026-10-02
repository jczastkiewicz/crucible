package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
)

// Mode$ CantSacrifice | ValidCard$ Card.Self (Hithlain Rope):
// the card is not a candidate for a sacrifice effect, so nothing is sacrificed.
func TestCantSacrificeSelfLeavesNoCandidate(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGameOn(t, scenarioDB(t))
	rope := g.NewCard(corpusCard(t, "Hithlain Rope"), p, engine.Battlefield)
	sba(g)
	if err := resolveWith(t, g, p, engine.NewScriptedController(), "DB$ Sacrifice | Defined$ You | SacValid$ Artifact"); err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if got := g.Card(rope).Zone; got != engine.Battlefield {
		t.Errorf("Hithlain Rope is in %v, want Battlefield: it can't be sacrificed", got)
	}
}

// Tajuru Preserver: spells and abilities an opponent controls can't make you
// sacrifice (ValidCause$ SpellAbility.OppCtrl, ForCost$ False); your own can.
func TestCantSacrificeToAnOpponentsAbilityButToYourOwn(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name   string
		caster func(p, other engine.PlayerID) engine.PlayerID
		gone   bool
	}{
		{"an opponent's edict", func(_, other engine.PlayerID) engine.PlayerID { return other }, false},
		{"your own sacrifice", func(p, _ engine.PlayerID) engine.PlayerID { return p }, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			g, p, other := newTwoPlayerGameOn(t, scenarioDB(t))
			g.NewCard(corpusCard(t, "Tajuru Preserver"), p, engine.Battlefield)
			victim := g.NewCard(corpusCard(t, "Grizzly Bears"), p, engine.Battlefield)
			sba(g)
			c := engine.NewScriptedController()
			c.QueueSacrificeChoice([]engine.CardID{victim})
			line := "DB$ Sacrifice | Defined$ Player | SacValid$ Creature.Bear"
			if err := resolveWith(t, g, tc.caster(p, other), c, line); err != nil {
				t.Fatalf("resolve: %v", err)
			}
			gone := g.Card(victim).Zone != engine.Battlefield
			if gone != tc.gone {
				t.Errorf("Grizzly Bears sacrificed = %v, want %v", gone, tc.gone)
			}
		})
	}
}

// Mode$ CantChangeLife (Platinum Emperion): a LoseLife effect loses nothing.
func TestCantChangeLifeStopsLifeLoss(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGameOn(t, scenarioDB(t))
	g.NewCard(corpusCard(t, "Platinum Emperion"), p, engine.Battlefield)
	sba(g)
	if err := resolveWith(t, g, p, engine.NewScriptedController(), "DB$ LoseLife | Defined$ You | LifeAmount$ 5"); err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if got := g.Player(p).Life; got != 20 {
		t.Errorf("life = %d, want 20", got)
	}
}
