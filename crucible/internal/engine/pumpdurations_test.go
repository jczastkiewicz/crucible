package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
)

// Each Duration$ registers its own end condition (addUntilCommand); the pump
// itself applies at once whichever way it later ends.
func TestPumpDurationsRegisterTheirEndCondition(t *testing.T) {
	t.Parallel()

	for _, duration := range []string{
		"UntilHostLeavesPlay", "UntilHostLeavesPlayOrEOT", "UntilHostLeavesPlayOrEndOfCombat",
		"UntilLoseControlOfHost", "UntilUnattached", "UntilTheEndOfYourNextTurn", "EndOfTurn",
	} {
		t.Run(duration, func(t *testing.T) {
			t.Parallel()
			g, p, _ := newTwoPlayerGame(t)
			host := resolveLine(t, g, p, engine.NewScriptedController(), "DB$ Pump | Defined$ Self | NumAtt$ 2 | Duration$ "+duration)
			if pw, _ := g.Card(host).Power(); pw != 3 {
				t.Errorf("power = %d, want 3", pw)
			}
		})
	}
}
