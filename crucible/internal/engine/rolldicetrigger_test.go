package engine_test

import (
	"strings"
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
)

// TestRollDiceRefusesWhileARolledDieTriggerIsLive proves a RollDice effect
// fails before any die is rolled while a Mode$ RolledDie or RolledDieOnce
// trigger is live: RollDiceEffect.java:320-336 fires both, and this port
// does not, so rolling would drop them silently (GO-7).
func TestRollDiceRefusesWhileARolledDieTriggerIsLive(t *testing.T) {
	t.Parallel()

	for _, mode := range []string{"RolledDie", "RolledDieOnce"} {
		t.Run(mode, func(t *testing.T) {
			t.Parallel()

			g, p, _ := newTwoPlayerGame(t)
			watcher := g.NewCard(planarDef(t, "Roll Watcher", "Creature Elf", []string{
				"Mode$ " + mode + " | ValidPlayer$ You | TriggerZones$ Battlefield | Execute$ Plus",
			}, nil, "Plus", "DB$ GainLife | Defined$ You | LifeAmount$ 1"), p, engine.Battlefield)

			err := castETBErr(t, g, p)
			if err == nil || !strings.Contains(err.Error(), "Mode$ "+mode) {
				t.Fatalf("err = %v, want Mode$ %s refused", err, mode)
			}
			g.Move(watcher, engine.Exile, p)
			if err := castETBErr(t, g, p); err != nil {
				t.Errorf("roll after the trigger left play: %v", err)
			}
		})
	}
}

func castETBErr(t *testing.T, g *engine.Game, p engine.PlayerID) error {
	t.Helper()
	_, err := castETBChain(t, g, p, etbChainDef(t, "Die Roller", "DB$ RollDice | Sides$ 6"), engine.NewScriptedController())
	return err
}
