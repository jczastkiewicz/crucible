package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
	"github.com/jczastkiewicz/crucible/internal/mana"
)

// Paragon of Modernity's Branch reads Count$ManaColorsPaid off its own
// activation (AbilityUtils.java:1791-1795): three colors of mana put a +1/+1
// counter on it, fewer give the +1/+1 pump.
func TestManaColorsPaidReadsTheActivationsColors(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name         string
		pool         []mana.Shard
		wantCounters int
		wantPower    int
	}{
		{"three colors", []mana.Shard{mana.ShardB, mana.ShardR, mana.ShardG}, 1, 3},
		{"one color", []mana.Shard{mana.ShardB, mana.ShardB, mana.ShardB}, 0, 3},
		{"two colors", []mana.Shard{mana.ShardB, mana.ShardB, mana.ShardR}, 0, 3},
	} {
		g, p, _ := newTwoPlayerGameOn(t, scenarioDB(t))
		paragon := g.NewCard(corpusCard(t, "Paragon of Modernity"), p, engine.Battlefield)
		c := engine.NewScriptedController()
		for _, shard := range tc.pool {
			g.Player(p).ManaPool.Add(map[mana.Shard]mana.Colors{mana.ShardB: mana.Black, mana.ShardR: mana.Red, mana.ShardG: mana.Green}[shard], 1)
			c.QueuePayGeneric(shard)
		}
		if !g.ActivateAbility(p, paragon, 0, c) {
			t.Fatalf("%s: could not activate", tc.name)
		}
		if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
			t.Fatalf("%s: ResolveStack: %v", tc.name, err)
		}
		if got := g.Card(paragon).Counters.Count(engine.CounterType("P1P1")); got != tc.wantCounters {
			t.Errorf("%s: +1/+1 counters = %d, want %d", tc.name, got, tc.wantCounters)
		}
		if pw, _ := g.Card(paragon).Power(); pw != tc.wantPower {
			t.Errorf("%s: power = %d, want %d", tc.name, pw, tc.wantPower)
		}
	}
}
