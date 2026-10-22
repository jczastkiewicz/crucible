package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
	"github.com/jczastkiewicz/crucible/internal/mana"
)

// TestActivationReadsRaiseAndReduceCostStatics proves CostAdjustment.adjust
// reaches an activated ability: Type$ Ability and an absent Type$ apply,
// Type$ Spell and a ValidSpell$ other than Activated do not, and a raise this
// port cannot add as mana refuses the activation rather than undercharging
// (GO-7).
func TestActivationReadsRaiseAndReduceCostStatics(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name string
		line string
		need int // green mana the "Cost$ 1" ability then takes; -1: never activates
	}{
		{"no static", "", 1},
		{"raise for abilities", "S:Mode$ RaiseCost | ValidCard$ Card | Type$ Ability | Amount$ 2", 3},
		{"raise without Type$", "S:Mode$ RaiseCost | ValidCard$ Card | Amount$ 1", 2},
		{"raise for spells only", "S:Mode$ RaiseCost | ValidCard$ Card | Type$ Spell | Amount$ 2", 1},
		{"raise for activated ValidSpell$", "S:Mode$ RaiseCost | ValidCard$ Card | ValidSpell$ Activated | Amount$ 1", 2},
		{"raise for mana abilities only", "S:Mode$ RaiseCost | ValidCard$ Card | ValidSpell$ Activated.ManaAbility | Amount$ 1", 1},
		{"reduce for abilities", "S:Mode$ ReduceCost | ValidCard$ Card | Type$ Ability | Amount$ 1", 0},
		{"non-mana raise refuses", "S:Mode$ RaiseCost | ValidCard$ Card | Type$ Ability | Cost$ PayLife<1>", -1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			for _, pool := range []int{tc.need - 1, tc.need} {
				if pool < 0 {
					continue
				}
				g, p, _ := newTwoPlayerGame(t)
				if tc.line != "" {
					g.NewCard(scriptDef(t, "Test Rule", "Enchantment", tc.line), p, engine.Battlefield)
				}
				host := g.NewCard(creatureDefWithAbility(t, "Actor", "AB$ GainLife | Cost$ 1 | Defined$ You | LifeAmount$ 1"), p, engine.Battlefield)
				g.Player(p).ManaPool.Add(mana.Green, pool)
				c := engine.NewScriptedController()
				for range max(tc.need, 1) + 1 {
					c.QueuePayGeneric(mana.ShardG)
				}
				got := g.ActivateAbility(p, host, 0, c)
				want := tc.need >= 0 && pool == tc.need
				if got != want {
					t.Errorf("activating with %d mana = %v, want %v", pool, got, want)
				}
			}
		})
	}
}
