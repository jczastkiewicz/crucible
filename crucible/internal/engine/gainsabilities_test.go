package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
)

// TestGainsAbilitiesOfCopiesActivatedAbilities proves GainsAbilitiesOf$
// (StaticAbilityContinuous.java:829-852): the host activates an ability it
// copied from another permanent, and GainsValidAbilities$ filters which ones.
func TestGainsAbilitiesOfCopiesActivatedAbilities(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, valid string
		want        int
	}{
		{"all activated", "", 21},
		{"mana abilities only", " | GainsValidAbilities$ Activated.ManaAbility", 20},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			g, p, _ := newTwoPlayerGame(t)
			g.NewCard(creatureDefWithAbility(t, "Test Source",
				"AB$ GainLife | Cost$ 0 | Defined$ You | LifeAmount$ 1"), p, engine.Battlefield)
			host := g.NewCard(continuousDef(t, "Test Copier", "Mode$ Continuous | Affected$ Card.Self | GainsAbilitiesOf$ Creature.Other"+tc.valid), p, engine.Battlefield)
			engine.CheckStateBasedActions(g, engine.NewScriptedController())
			if got := activateN(g, p, host, 1); (got == 1) != (tc.want == 21) {
				t.Fatalf("activations = %d, want life %d", got, tc.want)
			}
			if err := g.ResolveStack(engine.NewRegistry(), engine.NewScriptedController()); err != nil {
				t.Fatal(err)
			}
			if got := g.Player(p).Life; got != tc.want {
				t.Errorf("life = %d, want %d", got, tc.want)
			}
		})
	}
}
