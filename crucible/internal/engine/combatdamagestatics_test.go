package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
)

// combatDamageDealtTo runs one unblocked attack by an attacker built from
// attackerLines (its S: lines) at power/toughness, with an optional second
// permanent on the attacker's side carrying hostLines, and returns the
// defending player's life lost.
func combatDamageDealtTo(t *testing.T, power, toughness string, attackerLines, hostLines []string) int {
	t.Helper()
	g, a, b := combatGame(t)
	def := scriptDef(t, "Test Attacker", "Creature Elf", attackerLines...)
	def.Faces[0].Power, def.Faces[0].Toughness = power, toughness
	attacker := g.NewCard(def, a, engine.Battlefield)
	if hostLines != nil {
		g.NewCard(scriptDef(t, "Test Anthem", "Enchantment", hostLines...), a, engine.Battlefield)
	}
	declareAttacking(t, g, attacker)
	bc := engine.NewScriptedController()
	bc.QueueBlocks(nil)
	declareBlockers(t, g, bc)
	g.DealCombatDamage(engine.NewScriptedController())
	return 20 - g.Player(b).Life
}

// Card.getNetCombatDamage (Card.java:4537): Mode$ CombatDamageToughness,
// AssignNoCombatDamage and CombatDamageNegatePower statics change how much
// combat damage a creature assigns, each through ValidCard$ and Condition$.
func TestStaticCombatDamageModesChangeWhatACreatureAssigns(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name                       string
		power, toughness           string
		attackerLines, anthemLines []string
		want                       int
	}{
		{"no static assigns its power", "1", "4", nil, nil, 1},
		{"own toughness static", "1", "4",
			[]string{"S:Mode$ CombatDamageToughness | ValidCard$ Card.Self"}, nil, 4},
		{"a toughness static on another permanent naming creatures you control", "1", "4",
			nil, []string{"S:Mode$ CombatDamageToughness | ValidCard$ Creature.YouCtrl"}, 4},
		{"a toughness static naming only opposing creatures", "1", "4",
			nil, []string{"S:Mode$ CombatDamageToughness | ValidCard$ Creature.OppCtrl"}, 1},
		{"Condition$ PlayerTurn holds on its controller's turn", "1", "4",
			nil, []string{"S:Mode$ CombatDamageToughness | Condition$ PlayerTurn | ValidCard$ Creature"}, 4},
		{"Condition$ NotPlayerTurn fails on its controller's turn", "1", "4",
			nil, []string{"S:Mode$ CombatDamageToughness | Condition$ NotPlayerTurn | ValidCard$ Creature"}, 1},
		{"AssignNoCombatDamage assigns nothing", "3", "3",
			[]string{"S:Mode$ AssignNoCombatDamage | ValidCard$ Card.Self"}, nil, 0},
		{"AssignNoCombatDamage beats a toughness static", "3", "5",
			[]string{"S:Mode$ AssignNoCombatDamage | ValidCard$ Card.Self", "S:Mode$ CombatDamageToughness | ValidCard$ Card.Self"}, nil, 0},
		{"CombatDamageNegatePower flips a negative power", "-2", "4",
			[]string{"S:Mode$ CombatDamageNegatePower | ValidCard$ Card.Self+powerLT0"}, nil, 2},
		{"a negative power without the static assigns nothing", "-2", "4", nil, nil, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if got := combatDamageDealtTo(t, tc.power, tc.toughness, tc.attackerLines, tc.anthemLines); got != tc.want {
				t.Errorf("defender lost %d life, want %d", got, tc.want)
			}
		})
	}
}
