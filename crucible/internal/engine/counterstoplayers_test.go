package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
)

// These tests cover counters a player gets: every effect that gives them goes
// through the AddCounter replacements and fires the counter triggers the way
// Player.addCounterInternal and GameEntityCounterTable.replaceCounterEffect
// do (Player.java:858-890).

// TestPoisonEffectRunsCounterReplacements proves a Poison effect's counters
// go through the AddCounter replacements: an opponent's Halving Season turns
// three into one, and Vorinclex, Monstrous Raider doubles the ones its
// controller gives.
func TestPoisonEffectRunsCounterReplacements(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		card         string
		theirOwnSide bool
		num          string
		want         int
	}{
		{"Halving Season", true, "3", 1},
		{"Vorinclex, Monstrous Raider", false, "2", 4},
	} {
		g, p, other := newTwoPlayerGameOn(t, scenarioDB(t))
		g.SetTurnState(1, p, engine.Main1)
		owner := p
		if tc.theirOwnSide {
			owner = other
		}
		g.NewCard(corpusCard(t, tc.card), owner, engine.Battlefield)
		if _, err := resolveNow(t, g, p, engine.NewScriptedController(), nil, "DB$ Poison | Defined$ Opponent | Num$ "+tc.num); err != nil {
			t.Fatalf("%s: %v", tc.card, err)
		}
		if got := g.Player(other).Counters.Count(engine.Poison); got != tc.want {
			t.Errorf("%s: poison = %d, want %d", tc.card, got, tc.want)
		}
	}
}

// TestPlayerCounterTriggersFireOnlyForPlayers proves Mode$ CounterAddedOnce
// with ValidPlayer$ fires when its player gets energy, with the amount as
// the trigger's Amount (Aether Revolt, Fabrication Module), and not when
// +1/+1 counters go on a creature; a ValidCard$ line does the reverse.
func TestPlayerCounterTriggersFireOnlyForPlayers(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name, line string
		wantPlayer int
		wantCard   int
	}{
		{"player", "Mode$ CounterAddedOnce | ValidPlayer$ You | TriggerZones$ Battlefield | Execute$ TrigGain", 2, 0},
		{"card", "Mode$ CounterAddedOnce | ValidCard$ Creature.YouCtrl | TriggerZones$ Battlefield | Execute$ TrigGain", 0, 3},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			g, p, _ := newTwoPlayerGame(t)
			g.SetTurnState(1, p, engine.Main1)
			g.NewCard(planarDef(t, "Test Watcher", "Enchantment", []string{tc.line}, nil,
				"TrigGain", "DB$ GainLife | Defined$ You | LifeAmount$ X", "X", "TriggerCount$Amount"), p, engine.Battlefield)
			bear := g.NewCard(creatureDefPT(t, "2", "2"), p, engine.Battlefield)
			ctl := engine.NewScriptedController()

			life := g.Player(p).Life
			if _, err := resolveNow(t, g, p, ctl, nil, "DB$ PutCounter | Defined$ You | CounterType$ ENERGY | CounterNum$ 2"); err != nil {
				t.Fatalf("energy: %v", err)
			}
			if got := g.Player(p).Life - life; got != tc.wantPlayer {
				t.Errorf("energy: gained %d, want %d", got, tc.wantPlayer)
			}
			if got := g.Player(p).Counters.Count(engine.Energy); got != 2 {
				t.Errorf("energy = %d, want 2", got)
			}

			life = g.Player(p).Life
			if _, err := resolveNow(t, g, p, ctl, []engine.EntityID{engine.CardEntity(bear)},
				"DB$ PutCounter | ValidTgts$ Creature | CounterType$ P1P1 | CounterNum$ 3"); err != nil {
				t.Fatalf("+1/+1: %v", err)
			}
			if got := g.Player(p).Life - life; got != tc.wantCard {
				t.Errorf("+1/+1: gained %d, want %d", got, tc.wantCard)
			}
		})
	}
}

// TestAllWillBeOneDealsTheCountersPlaced proves All Will Be One's "deals that
// much damage" reads the counters placed, on a creature and on a player
// (CounterPlayerAddedAll's Amount).
func TestAllWillBeOneDealsTheCountersPlaced(t *testing.T) {
	t.Parallel()

	g, p, other := newTwoPlayerGameOn(t, scenarioDB(t))
	g.SetTurnState(1, p, engine.Main1)
	g.NewCard(corpusCard(t, "All Will Be One"), p, engine.Battlefield)
	bear := g.NewCard(creatureDefPT(t, "2", "2"), p, engine.Battlefield)
	ctl := engine.NewScriptedController()

	life := g.Player(other).Life
	ctl.QueueTargets([]engine.EntityID{engine.PlayerEntity(other)})
	if _, err := resolveNow(t, g, p, ctl, nil, "DB$ PutCounter | Defined$ You | CounterType$ ENERGY | CounterNum$ 2"); err != nil {
		t.Fatalf("energy: %v", err)
	}
	if got := life - g.Player(other).Life; got != 2 {
		t.Errorf("after 2 energy: opponent lost %d, want 2", got)
	}

	life = g.Player(other).Life
	ctl.QueueTargets([]engine.EntityID{engine.PlayerEntity(other)})
	if _, err := resolveNow(t, g, p, ctl, []engine.EntityID{engine.CardEntity(bear)},
		"DB$ PutCounter | ValidTgts$ Creature | CounterType$ P1P1 | CounterNum$ 3"); err != nil {
		t.Fatalf("+1/+1: %v", err)
	}
	if got := life - g.Player(other).Life; got != 3 {
		t.Errorf("after 3 +1/+1 counters: opponent lost %d, want 3", got)
	}
}
