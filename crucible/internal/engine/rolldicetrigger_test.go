package engine_test

import (
	"strings"
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
)

// TestRolledDieTriggersFireAfterARoll proves RollDiceEffect.java:320-336:
// Mode$ RolledDie fires once per kept die, RolledDieOnce once per roll, and
// Number$ counts the player's dice this turn.
func TestRolledDieTriggersFireAfterARoll(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		trigger string
		roll    string
		gain    int
	}{
		{"PerDie", "Mode$ RolledDie | ValidPlayer$ You | TriggerZones$ Battlefield | Execute$ Plus", "DB$ RollDice | Sides$ 6 | Amount$ 3", 3},
		{"Once", "Mode$ RolledDieOnce | ValidPlayer$ You | TriggerZones$ Battlefield | Execute$ Plus", "DB$ RollDice | Sides$ 6 | Amount$ 3", 1},
		{"NumberTwo", "Mode$ RolledDie | ValidPlayer$ You | Number$ 2 | TriggerZones$ Battlefield | Execute$ Plus", "DB$ RollDice | Sides$ 6 | Amount$ 3", 1},
		{"WrongSides", "Mode$ RolledDie | ValidPlayer$ You | ValidSides$ 20 | TriggerZones$ Battlefield | Execute$ Plus", "DB$ RollDice | Sides$ 6 | Amount$ 3", 0},
		{"OpponentOnly", "Mode$ RolledDie | ValidPlayer$ Opponent | TriggerZones$ Battlefield | Execute$ Plus", "DB$ RollDice | Sides$ 6 | Amount$ 3", 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			g, p, _ := newTwoPlayerGame(t)
			g.NewCard(planarDef(t, "Roll Watcher", "Creature Elf", []string{tt.trigger},
				nil, "Plus", "DB$ GainLife | Defined$ You | LifeAmount$ 1"), p, engine.Battlefield)

			if err := castETBErr(t, g, p, tt.roll); err != nil {
				t.Fatalf("roll: %v", err)
			}
			if got := g.Player(p).Life - 20; got != tt.gain {
				t.Errorf("life gained = %d, want %d", got, tt.gain)
			}
		})
	}
}

// TestRollDiceRefusesARolledDieTriggerItCannotRead proves a RolledDie line
// with a param this port does not read (ValidSA$) fails the roll before any
// die is rolled, and rolls again once the host leaves play (GO-7).
func TestRollDiceRefusesARolledDieTriggerItCannotRead(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGame(t)
	watcher := g.NewCard(planarDef(t, "Roll Watcher", "Creature Elf", []string{
		"Mode$ RolledDie | ValidPlayer$ You | ValidSA$ Spell | TriggerZones$ Battlefield | Execute$ Plus",
	}, nil, "Plus", "DB$ GainLife | Defined$ You | LifeAmount$ 1"), p, engine.Battlefield)

	err := castETBErr(t, g, p, "DB$ RollDice | Sides$ 6")
	if err == nil || !strings.Contains(err.Error(), "Mode$ RolledDie") {
		t.Fatalf("err = %v, want Mode$ RolledDie refused", err)
	}
	g.Move(watcher, engine.Exile, p)
	if err := castETBErr(t, g, p, "DB$ RollDice | Sides$ 6"); err != nil {
		t.Errorf("roll after the trigger left play: %v", err)
	}
}

func castETBErr(t *testing.T, g *engine.Game, p engine.PlayerID, line string) error {
	t.Helper()
	_, err := castETBChain(t, g, p, etbChainDef(t, "Die Roller", line), engine.NewScriptedController())
	return err
}
