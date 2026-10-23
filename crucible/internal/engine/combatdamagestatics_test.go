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

// Combat.assignAttackersDamage (Combat.java:805-812): a blocked attacker whose
// Optional$ AssignCombatDamageAsUnblocked static its controller accepts
// assigns all its damage to the player it attacks; a mandatory one does so
// unasked; declining leaves the blocker taking it.
func TestAssignCombatDamageAsUnblocked(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name         string
		line         string
		answers      []bool
		wantDefender int
		wantBlocker  int
	}{
		{"optional accepted", "S:Mode$ AssignCombatDamageAsUnblocked | ValidCard$ Card.Self | Optional$ True", []bool{true}, 3, 0},
		{"optional declined", "S:Mode$ AssignCombatDamageAsUnblocked | ValidCard$ Card.Self | Optional$ True", []bool{false}, 0, 3},
		{"mandatory is not asked", "S:Mode$ AssignCombatDamageAsUnblocked | ValidCard$ Card.Self", nil, 3, 0},
		{"another creature's static", "S:Mode$ AssignCombatDamageAsUnblocked | ValidCard$ Creature.Other", nil, 0, 3},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			g, a, b := combatGame(t)
			def := scriptDef(t, "Test Attacker", "Creature Elf", tc.line)
			def.Faces[0].Power, def.Faces[0].Toughness = "3", "5"
			attacker := g.NewCard(def, a, engine.Battlefield)
			blockerDef := creatureDefPT(t, "0", "9")
			blocker := g.NewCard(blockerDef, b, engine.Battlefield)
			declareAttacking(t, g, attacker)
			bc := engine.NewScriptedController()
			bc.QueueBlocks([]engine.Block{{Blocker: blocker, Attacker: attacker}})
			declareBlockers(t, g, bc)
			c := engine.NewScriptedController()
			for _, v := range tc.answers {
				c.QueueConfirmEffect(v)
			}

			g.DealCombatDamage(c)

			if got := 20 - g.Player(b).Life; got != tc.wantDefender {
				t.Errorf("defender lost %d, want %d", got, tc.wantDefender)
			}
			if got := g.Card(blocker).Damage.Marked; got != tc.wantBlocker {
				t.Errorf("blocker damage %d, want %d", got, tc.wantBlocker)
			}
		})
	}
}

// StaticAbilityCantAttach (StaticAbilityCantAttach.java:24-49): an Aura with
// "can be attached only to a creature with power 3 or greater" (Mode$
// CantAttach, ValidCard$ Card.Self, Target$ Creature.powerLT3) does not attach
// to a weaker creature.
func TestCantAttachStaticRefusesTheHost(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		power string
		want  bool
	}{{"2", false}, {"3", true}} {
		t.Run("power"+tc.power, func(t *testing.T) {
			t.Parallel()

			g, p, _ := newTwoPlayerGame(t)
			host := g.NewCard(creatureDefPT(t, tc.power, "3"), p, engine.Battlefield)
			aura := g.NewCard(scriptDef(t, "Test Naginata Aura", "Enchantment Aura",
				"K:Enchant:Creature",
				"S:Mode$ CantAttach | ValidCard$ Card.Self | Target$ Creature.powerLT3"), p, engine.Hand)
			c := engine.NewScriptedController()
			c.QueueTargets([]engine.EntityID{engine.CardEntity(host)})

			if !g.CastSpell(p, aura, c) {
				t.Fatal("CastSpell refused the Aura")
			}
			if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
				t.Fatalf("ResolveStack: %v", err)
			}
			if _, got := g.Card(aura).AttachedTo(); got != tc.want {
				t.Errorf("attached = %v, want %v", got, tc.want)
			}
		})
	}
}
