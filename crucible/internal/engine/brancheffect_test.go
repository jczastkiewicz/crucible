package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
)

// TestBranchEffectPicksSubAbilityByCondition proves BranchEffect.java: the
// compare picks TrueSubAbility$ or FalseSubAbility$, which resolves through
// the same Registry (additional.go).
func TestBranchEffectPicksSubAbilityByCondition(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		compare  string
		wantLife int
	}{{"GE2", 22}, {"GE5", 17}} {
		g, p, _ := newTwoPlayerGame(t)
		c := engine.NewScriptedController()
		def := etbChainDef(t, "Test Branch",
			"DB$ Branch | BranchConditionSVar$ 3 | BranchConditionSVarCompare$ "+tc.compare+" | TrueSubAbility$ DBGain | FalseSubAbility$ DBLose",
			"DBGain", "DB$ GainLife | Defined$ You | LifeAmount$ 2",
			"DBLose", "DB$ LoseLife | Defined$ You | LifeAmount$ 3")
		if _, err := castETBChain(t, g, p, def, c); err != nil {
			t.Fatalf("%s: ResolveStack: %v", tc.compare, err)
		}
		if got := g.Player(p).Life; got != tc.wantLife {
			t.Errorf("%s: life = %d, want %d", tc.compare, got, tc.wantLife)
		}
	}
}

// TestBranchEffectRejectsUnresolvableSVar proves an unresolvable condition
// fails closed rather than silently taking a branch.
func TestBranchEffectRejectsUnresolvableSVar(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGame(t)
	c := engine.NewScriptedController()
	def := etbChainDef(t, "Test Branch Bad", "DB$ Branch | BranchConditionSVar$ Bogus | TrueSubAbility$ DBGain",
		"DBGain", "DB$ GainLife | Defined$ You | LifeAmount$ 2")
	if _, err := castETBChain(t, g, p, def, c); err == nil {
		t.Fatal("ResolveStack succeeded, want an error")
	}
}
