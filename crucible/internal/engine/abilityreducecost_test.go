package engine_test

import (
	"strings"
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
	"github.com/jczastkiewicz/crucible/internal/mana"
)

// An activated ability's own ReduceCost$ takes generic mana off its cost
// (CostAdjustment.java:214-222).
func TestAbilityReduceCostLowersTheGenericPart(t *testing.T) {
	t.Parallel()
	g, p, _ := newTwoPlayerGame(t)
	g.SetTurnState(1, p, engine.Main1)
	host := g.NewCard(creatureDefWithAbility(t, "Discounted", "AB$ GainLife | Cost$ 3 | ReduceCost$ 2 | Defined$ You | LifeAmount$ 2"), p, engine.Battlefield)
	g.Player(p).ManaPool.Add(mana.Green, 1)
	c := engine.NewScriptedController()
	c.QueuePayGeneric(mana.ShardG)
	if !g.ActivateAbility(p, host, 0, c) {
		t.Fatalf("activation with one mana failed: %v", g.TakePendingError())
	}
}

// ReduceAmount$ repeats the ReduceCost$ mana cost that many times.
func TestAbilityReduceAmountRepeatsTheReducedCost(t *testing.T) {
	t.Parallel()
	g, p, _ := newTwoPlayerGame(t)
	g.SetTurnState(1, p, engine.Main1)
	host := g.NewCard(creatureDefWithAbility(t, "Repeat", "AB$ GainLife | Cost$ 4 | ReduceCost$ 2 | ReduceAmount$ 2 | Defined$ You | LifeAmount$ 2"), p, engine.Battlefield)
	c := engine.NewScriptedController()
	if !g.ActivateAbility(p, host, 0, c) {
		t.Fatalf("free activation failed: %v", g.TakePendingError())
	}
}

// An amount that cannot be evaluated refuses the activation, loudly.
func TestAbilityReduceCostUnresolvableAmountRefuses(t *testing.T) {
	t.Parallel()
	g, p, _ := newTwoPlayerGame(t)
	g.SetTurnState(1, p, engine.Main1)
	host := g.NewCard(creatureDefWithAbility(t, "Bogus", "AB$ GainLife | Cost$ 3 | ReduceCost$ Bogus | Defined$ You | LifeAmount$ 2"), p, engine.Battlefield)
	g.Player(p).ManaPool.Add(mana.Green, 3)
	if g.ActivateAbility(p, host, 0, engine.NewScriptedController()) {
		t.Fatal("activated with an unresolvable ReduceCost$")
	}
	if err := g.TakePendingError(); err == nil || !strings.Contains(err.Error(), "ReduceCost") {
		t.Errorf("pending error = %v, want the ReduceCost refusal", err)
	}
}
