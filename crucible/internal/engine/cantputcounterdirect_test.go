package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
)

// A counter placed straight on the entity (an AddCounter cost, Poison) obeys
// Mode$ CantPutCounter like any other (GameEntity.addCounter,
// CostPutCounter.canPay): the cost cannot be paid, the poison never lands.
func TestCantPutCounterDirectPlacements(t *testing.T) {
	t.Parallel()

	t.Run("AddCounter cost cannot be paid", func(t *testing.T) {
		t.Parallel()
		g, p, _ := newTwoPlayerGame(t)
		g.NewCard(scriptDef(t, "Test Solemnity", "Enchantment", "S:Mode$ CantPutCounter | ValidCard$ Creature"), p, engine.Battlefield)
		def := scriptDef(t, "Test Grower", "Creature Elf", "A:AB$ GainLife | Cost$ AddCounter<1/P1P1> | LifeAmount$ 1")
		grower := g.NewCard(def, p, engine.Battlefield)
		if g.ActivateAbility(p, grower, 0, engine.NewScriptedController()) {
			t.Error("ActivateAbility paid an AddCounter cost the creature cannot receive")
		}
	})
	t.Run("AddCounter cost without the static", func(t *testing.T) {
		t.Parallel()
		g, p, _ := newTwoPlayerGame(t)
		def := scriptDef(t, "Test Grower", "Creature Elf", "A:AB$ GainLife | Cost$ AddCounter<1/P1P1> | LifeAmount$ 1")
		grower := g.NewCard(def, p, engine.Battlefield)
		if !g.ActivateAbility(p, grower, 0, engine.NewScriptedController()) || g.Card(grower).Counters.Count(engine.P1P1) != 1 {
			t.Error("ActivateAbility refused a payable AddCounter cost")
		}
	})
	for _, tc := range []struct {
		name   string
		static string
		want   int
	}{
		{"poison blocked", "S:Mode$ CantPutCounter | ValidPlayer$ Player | CounterType$ POISON", 0},
		{"another counter type", "S:Mode$ CantPutCounter | ValidPlayer$ Player | CounterType$ ENERGY", 2},
		{"no static", "", 2},
	} {
		t.Run("Poison effect "+tc.name, func(t *testing.T) {
			t.Parallel()
			g, p, _ := newTwoPlayerGame(t)
			if tc.static != "" {
				g.NewCard(scriptDef(t, "Test Solemnity", "Enchantment", tc.static), p, engine.Battlefield)
			}
			def := scriptDef(t, "Test Venom", "Creature Elf", "A:AB$ Poison | Cost$ T | Defined$ You | Num$ 2")
			venom := g.NewCard(def, p, engine.Battlefield)
			g.Card(venom).SummonSick = false
			c := engine.NewScriptedController()
			if !g.ActivateAbility(p, venom, 0, c) {
				t.Fatal("ActivateAbility failed")
			}
			if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
				t.Fatalf("ResolveStack: %v", err)
			}
			if got := g.Player(p).Counters.Count(engine.Poison); got != tc.want {
				t.Errorf("poison counters = %d, want %d", got, tc.want)
			}
		})
	}
}
