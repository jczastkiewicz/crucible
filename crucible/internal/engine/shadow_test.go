package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
)

// TestShadowCreaturesBlockAndAreBlockedOnlyByShadow proves CR 702.28: a
// creature with shadow can be blocked only by creatures with shadow, and can
// block only creatures with shadow (CardFactoryUtil.java:3994-4002).
func TestShadowCreaturesBlockAndAreBlockedOnlyByShadow(t *testing.T) {
	t.Parallel()

	g, a, b := combatGame(t)
	shadowAttacker := g.NewCard(creatureDefPTKeywords(t, "2", "2", "Shadow"), a, engine.Battlefield)
	plainAttacker := g.NewCard(creatureDefPT(t, "2", "2"), a, engine.Battlefield)
	shadowBlocker := g.NewCard(creatureDefPTKeywords(t, "2", "2", "Shadow"), b, engine.Battlefield)
	plainBlocker := g.NewCard(creatureDefPT(t, "2", "2"), b, engine.Battlefield)

	for _, tc := range []struct {
		name              string
		attacker, blocker engine.CardID
		want              bool
	}{
		{"plain blocks plain", plainAttacker, plainBlocker, true},
		{"shadow blocks shadow", shadowAttacker, shadowBlocker, true},
		{"plain blocks shadow", shadowAttacker, plainBlocker, false},
		{"shadow blocks plain", plainAttacker, shadowBlocker, false},
	} {
		if got := g.CanBlock(tc.attacker, tc.blocker); got != tc.want {
			t.Errorf("%s: CanBlock = %v, want %v", tc.name, got, tc.want)
		}
	}
}

// TestCanBlockIfShadowLetsTheWallBlockShadow proves Mode$ CanBlockIfShadow
// (Heartwood Dryad's shape): the creature naming itself as ValidBlocker$ may
// block a shadow attacker, other plain creatures still may not.
func TestCanBlockIfShadowLetsTheWallBlockShadow(t *testing.T) {
	t.Parallel()

	g, a, b := combatGame(t)
	shadowAttacker := g.NewCard(creatureDefPTKeywords(t, "2", "2", "Shadow"), a, engine.Battlefield)
	wall := g.NewCard(combatCreatureDef(t, "Test Dryad",
		[]string{"Mode$ CanBlockIfShadow | ValidAttacker$ Creature.withShadow | ValidBlocker$ Card.Self"}, nil), b, engine.Battlefield)
	plain := g.NewCard(creatureDefPT(t, "2", "2"), b, engine.Battlefield)

	if !g.CanBlock(shadowAttacker, wall) {
		t.Error("the CanBlockIfShadow creature cannot block a shadow attacker")
	}
	if g.CanBlock(shadowAttacker, plain) {
		t.Error("a plain creature blocks a shadow attacker beside the dryad")
	}
}
