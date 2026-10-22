package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
)

// TestConditionNotPresentAndOpponentTurn proves SpellAbilityCondition.java:
// 139-140 and :196-198: ConditionNotPresent$ is a presence count forced to
// EQ0 (a ConditionCompare$ beside it is ignored), ConditionOpponentTurn$
// needs an opponent to be the active player.
func TestConditionNotPresentAndOpponentTurn(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name     string
		line     string
		creature bool
		want     int
	}{
		{"not present, none", "ConditionNotPresent$ Creature.YouCtrl+Other", false, 21},
		{"not present, one", "ConditionNotPresent$ Creature.YouCtrl+Other", true, 20},
		{"compare ignored", "ConditionNotPresent$ Creature.YouCtrl+Other | ConditionCompare$ GE1", true, 20},
		{"opponent turn, own turn", "ConditionOpponentTurn$ True", false, 20},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			g, p, _ := newTwoPlayerGame(t)
			if tc.creature {
				g.NewCard(creatureDef(t), p, engine.Battlefield)
			}
			resolveLine(t, g, p, engine.NewScriptedController(),
				"DB$ GainLife | Defined$ You | LifeAmount$ 1 | "+tc.line)
			if got := g.Player(p).Life; got != tc.want {
				t.Errorf("life = %d, want %d", got, tc.want)
			}
		})
	}
}
