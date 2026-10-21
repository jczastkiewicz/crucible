package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
	"github.com/jczastkiewicz/crucible/internal/mana"
)

// Spree and Tiered: the ModeCost$ of each chosen mode joins the spell's cost
// (CostAdjustment.java:143-152), so a mode the pool cannot pay for is not cast
// and a paid one resolves.
func TestCharmModeCostJoinsTheCastCost(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name  string
		modes []int
		pool  int
		want  bool
		life  int
	}{
		{"free mode", []int{0}, 0, true, 21},
		{"paid mode with the mana", []int{1}, 1, true, 22},
		{"paid mode without the mana", []int{1}, 0, false, 20},
		{"both modes", []int{0, 1}, 1, true, 23},
		{"a non-mana mode cost adds nothing", []int{2}, 0, true, 24},
	} {
		g, p, _ := newTwoPlayerGameOn(t, scenarioDB(t))
		g.Player(p).Life = 20
		spell := g.NewCard(copyTestDef(t, "Test Spree", "Instant", "", "",
			"Cost:G", "A:SP$ Charm | Choices$ DBA,DBB,DBC | MinCharmNum$ 1 | CharmNum$ 2",
			"SVar:DBA:DB$ GainLife | Defined$ You | LifeAmount$ 1 | ModeCost$ 0",
			"SVar:DBB:DB$ GainLife | Defined$ You | LifeAmount$ 2 | ModeCost$ 1",
			"SVar:DBC:DB$ GainLife | Defined$ You | LifeAmount$ 4 | ModeCost$ Discard<1/Card>"), p, engine.Hand)
		g.Player(p).ManaPool.Add(mana.Green, 1)
		g.Player(p).ManaPool.AddColorless(tc.pool)
		c := engine.NewScriptedController()
		c.QueueModeChoice(tc.modes)
		for range max(tc.pool, 1) {
			c.QueuePayGeneric(mana.ShardC)
		}
		if got := g.CastSpell(p, spell, c); got != tc.want {
			t.Errorf("%s: cast = %v, want %v", tc.name, got, tc.want)
			continue
		}
		if !tc.want {
			continue
		}
		if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}
		if got := g.Player(p).Life; got != tc.life {
			t.Errorf("%s: life = %d, want %d", tc.name, got, tc.life)
		}
	}
}
