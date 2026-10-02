package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
)

// Mode$ CantPreventDamage (Card.canDamagePrevented): a static naming the source
// (and IsCombat$, ValidSource$) makes a damage prevention replacement skip it.
func TestCantPreventDamageOverridesPrevention(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name string
		line string
		want int // damage the defender takes from a 3-power unblocked attacker
	}{
		{"prevention applies", "", 0},
		{"damage can't be prevented", "S:Mode$ CantPreventDamage", 3},
		{"combat damage can't be prevented", "S:Mode$ CantPreventDamage | IsCombat$ True", 3},
		{"only noncombat damage can't be prevented", "S:Mode$ CantPreventDamage | IsCombat$ False", 0},
		{"only the attacker's own damage", "S:Mode$ CantPreventDamage | ValidSource$ Card.Self", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			g, a, b := combatGame(t)
			g.NewCard(scriptDef(t, "Test Fog Bank", "Enchantment",
				"R:Event$ DamageDone | Prevent$ True | IsCombat$ True | ActiveZones$ Battlefield"), b, engine.Battlefield)
			if tc.line != "" {
				g.NewCard(scriptDef(t, "Test Torment", "Enchantment", tc.line), a, engine.Battlefield)
			}
			unblockedHit(t, g, g.NewCard(creatureDefPT(t, "3", "3"), a, engine.Battlefield))
			if got := 20 - g.Player(b).Life; got != tc.want {
				t.Errorf("defender took %d, want %d", got, tc.want)
			}
		})
	}
}
