package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
)

// Targeted$AllTypes counts the distinct core types, supertypes and subtypes
// among the targets, summed (AbilityUtils.java:3800-3803): Embiggen on a
// Legendary Artifact Creature -- Golem Soldier is 3 + 1 + 2.
func TestTargetedAllTypesCountsEveryTypeKind(t *testing.T) {
	t.Parallel()
	g, p, _ := newTwoPlayerGame(t)
	target := g.NewCard(scriptDef(t, "Typed", "Legendary Artifact Creature Golem Soldier"), p, engine.Battlefield)
	if _, err := resolveNow(t, g, p, engine.NewScriptedController(), []engine.EntityID{engine.CardEntity(target)},
		"DB$ Pump | ValidTgts$ Creature | NumAtt$ +X | Duration$ EndOfTurn", "X", "Targeted$AllTypes"); err != nil {
		t.Fatal(err)
	}
	if got, _ := g.Card(target).Power(); got != 6 {
		t.Errorf("power = %d, want 6 (3 core + 1 super + 2 sub on a 0-power base)", got)
	}
}
