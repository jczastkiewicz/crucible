package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
)

// A RolledDie trigger naming RolledToVisitAttractions$ (Ferris Wheel's comeback)
// fires for a roll made with ToVisitYourAttractions$ and for no other roll.
func TestRolledToVisitAttractionsTriggerFiresOnlyForTheVisitRoll(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name, roll string
		want       int
	}{
		{"visit roll", "DB$ RollDice | Sides$ 6 | ToVisitYourAttractions$ True", 21},
		{"ordinary roll", "DB$ RollDice | Sides$ 6", 20},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			g, p, _ := newTwoPlayerGame(t)
			g.NewCard(planarDef(t, "Watcher", "Artifact", []string{
				"Mode$ RolledDie | RolledToVisitAttractions$ True | ValidPlayer$ You | TriggerZones$ Battlefield | Execute$ Gain",
			}, nil, "Gain", "DB$ GainLife | Defined$ You | LifeAmount$ 1"), p, engine.Battlefield)
			resolveLine(t, g, p, engine.NewScriptedController(), tc.roll)
			if got := g.Player(p).Life; got != tc.want {
				t.Errorf("life = %d, want %d", got, tc.want)
			}
		})
	}
}
