package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
)

// TestActivateAbilityWithNoLegalTargetPaysNothing proves CR 602.2b / 601.2c:
// targets are chosen before costs are paid (PlaySpellAbility.java:675-683), so
// an ability whose ValidTgts$ has no legal target is not activated and its
// {T}, life and sacrifice costs are never paid.
func TestActivateAbilityWithNoLegalTargetPaysNothing(t *testing.T) {
	t.Parallel()

	for _, cost := range []string{"T", "T PayLife<2>", "Sac<1/CARDNAME>"} {
		t.Run(cost, func(t *testing.T) {
			t.Parallel()

			g, p, _ := newTwoPlayerGame(t)
			def := creatureDefWithAbility(t, "Test Tapped Killer",
				"AB$ Destroy | Cost$ "+cost+" | ValidTgts$ Creature.tapped | TgtPrompt$ Select target tapped creature")
			killer := g.NewCard(def, p, engine.Battlefield)
			life := g.Player(p).Life

			if g.ActivateAbility(p, killer, 0, engine.NewScriptedController()) {
				t.Fatal("ActivateAbility succeeded with no tapped creature to target")
			}
			c := g.Card(killer)
			if c.Zone != engine.Battlefield || c.Tapped {
				t.Errorf("source zone %v tapped %v after a refused activation, want untapped on the battlefield", c.Zone, c.Tapped)
			}
			if got := g.Player(p).Life; got != life {
				t.Errorf("life = %d, want %d: the life cost was paid for nothing", got, life)
			}
			if g.StackLen() != 0 {
				t.Errorf("stack size = %d, want 0", g.StackLen())
			}
		})
	}
}
