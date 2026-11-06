package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
)

// Weight of Conscience: "Tap two untapped creatures you control that share a
// creature type: Exile enchanted creature" (tapXType<2/Creature.
// sharesCreatureTypeWith>, CostTapType.java:174-181). Two Bears pay it; a
// Bear and an Angel do not, and a Bear-and-Angel pick is replaced by the two
// Bears.
func TestTapTwoCreaturesThatShareACreatureType(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name      string
		twoBears  bool
		pickAngel bool
		want      bool
	}{
		{"no shared type", false, false, false},
		{"two Bears", true, false, true},
		{"a pick without a shared type", true, true, true},
	} {
		g, p, other := newTwoPlayerGameOn(t, scenarioDB(t))
		g.SetTurnState(1, p, engine.Main1)
		victim := g.NewCard(corpusCard(t, "Serra Angel"), other, engine.Battlefield)
		aura := g.NewCard(corpusCard(t, "Weight of Conscience"), p, engine.Battlefield)
		g.Attach(aura, victim)
		bear := g.NewCard(corpusCard(t, "Grizzly Bears"), p, engine.Battlefield)
		angel := g.NewCard(corpusCard(t, "Serra Angel"), p, engine.Battlefield)
		var bear2 engine.CardID
		if tc.twoBears {
			bear2 = g.NewCard(corpusCard(t, "Grizzly Bears"), p, engine.Battlefield)
		}
		c := engine.NewScriptedController()
		pick := []engine.CardID{bear, bear2}
		if tc.pickAngel {
			pick = []engine.CardID{bear, angel}
		}
		c.QueueTapChoice(pick)
		if got := g.ActivateAbility(p, aura, 0, c); got != tc.want {
			t.Fatalf("%s: activated = %v, want %v (%v)", tc.name, got, tc.want, g.TakePendingError())
		}
		if !tc.want {
			continue
		}
		if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
			t.Fatal(err)
		}
		if z := g.Card(victim).Zone; z != engine.Exile {
			t.Errorf("%s: enchanted creature in %v, want Exile", tc.name, z)
		}
		if !g.Card(bear).Tapped || !g.Card(bear2).Tapped || g.Card(angel).Tapped {
			t.Errorf("%s: tapped bear %v, bear2 %v, angel %v; want the two Bears", tc.name,
				g.Card(bear).Tapped, g.Card(bear2).Tapped, g.Card(angel).Tapped)
		}
	}
}
