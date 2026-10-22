package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
)

// TestBoastNeedsAnAttackAndIsOncePerTurn proves Boast$ (SpellAbilityEffect.
// java:85-88, SpellAbilityRestriction.java:418-435, :461-463): refused until
// the host attacked this turn, then allowed once.
func TestBoastNeedsAnAttackAndIsOncePerTurn(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGame(t)
	boaster := g.NewCard(creatureDefWithAbility(t, "Test Boaster",
		"AB$ GainLife | Cost$ 0 | Defined$ You | LifeAmount$ 1 | Boast$ True"), p, engine.Battlefield)
	if activateN(g, p, boaster, 1) != 0 {
		t.Fatal("boast activated before the creature attacked")
	}
	declareLoneAttacker(t, g, boaster)
	if got := activateN(g, p, boaster, 3); got != 1 {
		t.Errorf("activations after attacking = %d, want 1", got)
	}
}

// TestActivationsStaticRaisesTheBoastLimit proves Mode$ Activations
// (StaticAbilityAdditionalActivations.java:15-41, Birgi's shape): MinLimit$ 2
// lets a boast ability activate twice in a turn, not three times.
func TestActivationsStaticRaisesTheBoastLimit(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGame(t)
	boaster := g.NewCard(creatureDefWithAbility(t, "Test Boaster",
		"AB$ GainLife | Cost$ 0 | Defined$ You | LifeAmount$ 1 | Boast$ True"), p, engine.Battlefield)
	g.NewCard(scriptDef(t, "Test Birgi", "Enchantment",
		"S:Mode$ Activations | ValidCard$ Creature.YouCtrl | ValidSA$ Activated.Boast | ValidPlayer$ You | MinLimit$ 2 | PlayerTurn$ You"), p, engine.Battlefield)
	declareLoneAttacker(t, g, boaster)
	if got := activateN(g, p, boaster, 3); got != 2 {
		t.Errorf("activations with MinLimit 2 = %d, want 2", got)
	}
}
