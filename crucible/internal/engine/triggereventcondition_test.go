package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
)

// A trigger's CheckSVar$ and CheckDefinedPlayer$ read the triggering objects
// of the event being checked (Trigger.checkWith, Trigger.java:428-446): "if no
// opponent has more life than that player" (Veteran Soldier) is
// CheckSVar$ TriggeredDefendingPlayer$LifeTotal | SVarCompare$
// GECount$OppGreatestLifeTotal, and "if it's attacking the player with the
// most life" (Scourge of the Throne) is CheckDefinedPlayer$
// TriggeredDefender.withMostLife. Attacking the higher-life opponent fires
// both; attacking the lower-life one fires neither.
func TestAttacksConditionReadsTheAttackedPlayer(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name      string
		condition string
	}{
		{"CheckSVar", "CheckSVar$ TriggeredDefendingPlayer$LifeTotal | SVarCompare$ GECount$OppGreatestLifeTotal"},
		{"CheckDefinedPlayer", "CheckDefinedPlayer$ TriggeredDefender.withMostLife"},
	} {
		for _, attackRich := range []bool{true, false} {
			g, p, rich, poor := newThreePlayerGame(t)
			g.Player(poor).Life = 10
			attacker := g.NewCard(scriptDef(t, "Test Attacker", "Creature Elf",
				"T:Mode$ Attacks | ValidCard$ Card.Self | Attacked$ Player | "+tc.condition+" | Execute$ TrigGain",
				"SVar:TrigGain:DB$ GainLife | Defined$ You | LifeAmount$ 5"), p, engine.Battlefield)
			defender := poor
			if attackRich {
				defender = rich
			}
			c := engine.NewScriptedController()
			c.QueueAttackers([]engine.CardID{attacker})
			c.QueueAttackTarget(engine.PlayerEntity(defender))
			declareAttackers(t, g, c)
			if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
				t.Fatalf("%s rich=%v: ResolveStack: %v", tc.name, attackRich, err)
			}
			want := 20
			if attackRich {
				want = 25
			}
			if got := g.Player(p).Life; got != want {
				t.Errorf("%s, attacking the higher-life player %v: life = %d, want %d", tc.name, attackRich, got, want)
			}
		}
	}
}

// Suppressor Skyguard's "if that player has another opponent who isn't being
// attacked": CheckDefinedPlayer$ Player.OpponentOf TriggeredAttackingPlayer+
// !Defending, read off the AttackersDeclared event (getDefinedPlayers' last
// branch filters every player, AbilityUtils.java:1187-1199). With a third
// player left unattacked it fires; with no other opponent it does not.
func TestAttackersDeclaredConditionReadsTheAttackingPlayer(t *testing.T) {
	t.Parallel()

	const line = "Mode$ AttackersDeclared | AttackingPlayer$ Player | AttackedTarget$ You | CheckDefinedPlayer$ Player.OpponentOf TriggeredAttackingPlayer+!Defending | TriggerZones$ Battlefield | Execute$ TrigGain"
	for _, players := range []int{3, 2} {
		var g *engine.Game
		var attacking, watching engine.PlayerID
		if players == 3 {
			g, attacking, watching, _ = newThreePlayerGame(t)
		} else {
			g, attacking, watching = newTwoPlayerGame(t)
		}
		g.NewCard(triggerWatcherDef(t, "Test Skyguard", line,
			"TrigGain", "DB$ GainLife | Defined$ You | LifeAmount$ 5"), watching, engine.Battlefield)
		attacker := g.NewCard(creatureDefPT(t, "2", "2"), attacking, engine.Battlefield)
		c := engine.NewScriptedController()
		c.QueueAttackers([]engine.CardID{attacker})
		c.QueueAttackTarget(engine.PlayerEntity(watching))
		declareAttackers(t, g, c)
		if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
			t.Fatalf("%d players: ResolveStack: %v", players, err)
		}
		want := 20
		if players == 3 {
			want = 25
		}
		if got := g.Player(watching).Life; got != want {
			t.Errorf("%d players: watcher's life = %d, want %d", players, got, want)
		}
	}
}

// Verrak, Warped Sengir's "if life was paid to activate it": CheckSVar$ X with
// X = TriggerCount$LifeAmount, the life the activation paid
// (TriggerSpellAbilityCastOrCopy.setTriggeringObjects), read at trigger time.
func TestAbilityCastConditionReadsTheLifePaid(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		cost string
		want int
	}{
		{"PayLife<2>", 20 - 2 + 1 + 3},
		{"0", 20 + 1},
	} {
		g, p, _ := newTwoPlayerGameOn(t, scenarioDB(t))
		g.SetTurnState(1, p, engine.Main1)
		g.NewCard(scriptDef(t, "Test Verrak", "Enchantment",
			"T:Mode$ AbilityCast | ValidActivatingPlayer$ You | ValidSA$ SpellAbility.!ManaAbility | CheckSVar$ X | TriggerZones$ Battlefield | Execute$ TrigGain",
			gainThreeSVar, "SVar:X:TriggerCount$LifeAmount"), p, engine.Battlefield)
		src := g.NewCard(creatureDefWithAbility(t, "Test Source", "AB$ GainLife | Cost$ "+tc.cost+" | Defined$ You | LifeAmount$ 1"), p, engine.Battlefield)
		g.Card(src).SummonSick = false
		sba(g)
		c := engine.NewScriptedController()
		if !g.ActivateAbility(p, src, 0, c) {
			t.Fatalf("Cost$ %s: activation failed", tc.cost)
		}
		if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
			t.Fatalf("Cost$ %s: ResolveStack: %v", tc.cost, err)
		}
		if got := g.Player(p).Life; got != tc.want {
			t.Errorf("Cost$ %s: life = %d, want %d", tc.cost, got, tc.want)
		}
	}
}

// Lolth, Spider Queen's emblem: "if that player lost less than 8 life this
// turn" is CheckSVar$ X | SVarCompare$ LT8 with X = TriggeredTarget$
// LifeLostThisTurn, the damaged player of the DamageDoneOnce event, read as
// combat damage is dealt. A player already down 5 life who takes 3 more sits
// at 8 and the trigger does not fire; a 2-power hit leaves them at 7 and it
// does.
func TestDamageDoneOnceConditionReadsTheDamagedPlayer(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		power string
		want  int
	}{
		{"3", 20 - 5 - 3},
		{"2", 20 - 5 - 2 - 5},
	} {
		g, p, other := newTwoPlayerGame(t)
		g.Player(other).Life = 15
		g.Player(other).LifeLostThisTurn = 5
		attacker := g.NewCard(creatureDefPT(t, tc.power, tc.power), p, engine.Battlefield)
		g.NewCard(triggerWatcherDef(t, "Test Lolth Emblem",
			"Mode$ DamageDoneOnce | ValidSource$ Creature.YouCtrl | ValidTarget$ Player | CombatDamage$ True | CheckSVar$ X | SVarCompare$ LT8 | TriggerZones$ Battlefield | Execute$ TrigLose",
			"TrigLose", "DB$ LoseLife | Defined$ TriggeredTarget | LifeAmount$ 5",
			"X", "TriggeredTarget$LifeLostThisTurn"), p, engine.Battlefield)

		combatHit(t, g, attacker)
		if got := g.Player(other).Life; got != tc.want {
			t.Errorf("power %s: damaged player's life = %d, want %d", tc.power, got, tc.want)
		}
	}
}
