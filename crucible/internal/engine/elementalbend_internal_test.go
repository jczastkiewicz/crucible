package engine

// package engine, not engine_test (TEST-2's "invariant not observable from
// outside" row): only Avatar Aang reads Count$AllFourBend, and no corpus line
// lets one test bend all four elements in a turn, so its true branch is
// reached by marking the bends directly.

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/expr"
)

// Count$AllFourBend.1.0 (AbilityUtils.java:2336-2337) is 1 once a player has
// airbent, earthbent, firebent and waterbent this turn, 0 with any missing.
func TestAllFourBendNeedsEveryElement(t *testing.T) {
	t.Parallel()

	g := targetCandidatesTestGame(t, "a", "b")
	p := g.Players()[0]
	host := g.NewCard(nil, p, Battlefield)
	count := expr.ParseCount("AllFourBend.1.0")
	for i, kind := range elementalBendKinds {
		if n, ok, _ := branchCountValue(g, nil, p, host, count, 0); !ok || n != 0 {
			t.Fatalf("after %d bends: Count$AllFourBend = (%d, %v), want 0", i, n, ok)
		}
		g.elementalBendMatches(p, kind)
	}
	if n, ok, _ := branchCountValue(g, nil, p, host, count, 0); !ok || n != 1 {
		t.Errorf("after all four bends: Count$AllFourBend = (%d, %v), want 1", n, ok)
	}
}
