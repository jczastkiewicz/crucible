package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
)

func activateN(g *engine.Game, p engine.PlayerID, card engine.CardID, n int) (ok int) {
	for i := 0; i < n; i++ {
		if g.ActivateAbility(p, card, 0, engine.NewScriptedController()) {
			ok++
		}
	}
	return ok
}

// TestActivationLimitCapsActivationsPerTurn proves ActivationLimit$ N allows
// N activations of the ability in a turn and declines the next
// (SpellAbilityRestriction.java:583-590), and the count resets at cleanup.
func TestActivationLimitCapsActivationsPerTurn(t *testing.T) {
	t.Parallel()

	for _, limit := range []int{1, 2, 3} {
		g, p, _ := newTwoPlayerGame(t)
		def := creatureDefWithAbility(t, "Test Limited",
			"AB$ GainLife | Cost$ 0 | Defined$ You | LifeAmount$ 1 | ActivationLimit$ "+string(rune('0'+limit)))
		creature := g.NewCard(def, p, engine.Battlefield)

		if got := activateN(g, p, creature, 5); got != limit {
			t.Fatalf("limit %d: activations = %d, want %d", limit, got, limit)
		}
		for g.ActivePhase() != engine.Cleanup {
			g.AdvancePhase(engine.NewScriptedController())
		}
		if got := activateN(g, p, creature, 5); got != limit {
			t.Errorf("limit %d: activations after cleanup = %d, want %d again", limit, got, limit)
		}
	}
}

// TestGameActivationLimitSurvivesCleanup proves GameActivationLimit$ counts
// the whole game: cleanup does not reset it, and each ability counts alone.
func TestGameActivationLimitSurvivesCleanup(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGame(t)
	def := creatureDefWithAbility(t, "Test Once Per Game",
		"AB$ GainLife | Cost$ 0 | Defined$ You | LifeAmount$ 1 | GameActivationLimit$ 1")
	creature := g.NewCard(def, p, engine.Battlefield)
	other := g.NewCard(def, p, engine.Battlefield)

	if got := activateN(g, p, creature, 3); got != 1 {
		t.Fatalf("activations = %d, want 1", got)
	}
	for g.ActivePhase() != engine.Cleanup {
		g.AdvancePhase(engine.NewScriptedController())
	}
	if got := activateN(g, p, creature, 3); got != 0 {
		t.Errorf("activations after cleanup = %d, want 0: the game limit is not a turn limit", got)
	}
	if got := activateN(g, p, other, 3); got != 1 {
		t.Errorf("a second permanent activated %d times, want its own 1", got)
	}
}

// TestActivationCountsResetWhenTheCardChangesZone proves a card that left and
// came back is a new object with no activations (CR 400.7).
func TestActivationCountsResetWhenTheCardChangesZone(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGame(t)
	def := creatureDefWithAbility(t, "Test Once Per Game",
		"AB$ GainLife | Cost$ 0 | Defined$ You | LifeAmount$ 1 | GameActivationLimit$ 1")
	creature := g.NewCard(def, p, engine.Battlefield)
	if got := activateN(g, p, creature, 2); got != 1 {
		t.Fatalf("activations = %d, want 1", got)
	}

	g.Move(creature, engine.Hand, p)
	g.Move(creature, engine.Battlefield, p)

	if got := activateN(g, p, creature, 2); got != 1 {
		t.Errorf("activations after returning = %d, want 1 again", got)
	}
}
