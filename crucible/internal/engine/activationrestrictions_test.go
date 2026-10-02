package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
	"github.com/jczastkiewicz/crucible/internal/mana"
)

// TestActivatorRestrictsWhoMayActivate proves Activator$ names the players
// who may activate (SpellAbilityRestriction.checkActivatorRestrictions):
// by default only the controller, with Activator$ Opponent only the others.
func TestActivatorRestrictsWhoMayActivate(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name       string
		line       string
		controller bool
		opponent   bool
	}{
		{"default", "AB$ GainLife | Cost$ 0 | Defined$ You | LifeAmount$ 1", true, false},
		{"Opponent", "AB$ GainLife | Cost$ 0 | Defined$ You | LifeAmount$ 1 | Activator$ Opponent", false, true},
		{"Player", "AB$ GainLife | Cost$ 0 | Defined$ You | LifeAmount$ 1 | Activator$ Player", true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			g, p, other := newTwoPlayerGame(t)
			card := g.NewCard(creatureDefWithAbility(t, "Test Activator", tc.line), p, engine.Battlefield)

			if got := g.ActivateAbility(p, card, 0, engine.NewScriptedController()); got != tc.controller {
				t.Errorf("controller activation = %v, want %v", got, tc.controller)
			}
			if got := g.ActivateAbility(other, card, 0, engine.NewScriptedController()); got != tc.opponent {
				t.Errorf("opponent activation = %v, want %v", got, tc.opponent)
			}
		})
	}
}

// TestActivationNeedsItsPresentCondition proves IsPresent$ with
// PresentCompare$ gates an activated ability on the board
// (SpellAbilityRestriction.java:417-433), default PresentCompare$ GE1.
func TestActivationNeedsItsPresentCondition(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name         string
		compare      string
		alone, mated bool // activation without / with another creature
	}{
		{"GE1", "", false, true},
		{"EQ0", " | PresentCompare$ EQ0", true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			g, p, _ := newTwoPlayerGame(t)
			card := g.NewCard(creatureDefWithAbility(t, "Test Needs Friend",
				"AB$ GainLife | Cost$ 0 | Defined$ You | LifeAmount$ 1 | IsPresent$ Creature.Other+YouCtrl"+tc.compare), p, engine.Battlefield)

			if got := g.ActivateAbility(p, card, 0, engine.NewScriptedController()); got != tc.alone {
				t.Errorf("activation with no other creature = %v, want %v", got, tc.alone)
			}
			g.NewCard(layer456Creature(t, "G"), p, engine.Battlefield)
			g.Card(card).Tapped = false
			if got := g.ActivateAbility(p, card, 0, engine.NewScriptedController()); got != tc.mated {
				t.Errorf("activation with another creature = %v, want %v", got, tc.mated)
			}
		})
	}
}

// TestActivationNeedsItsPlayerState proves Activation$ Threshold, LifeTotal$
// with LifeAmount$ and CheckSVar$ with SVarCompare$ each gate activation.
func TestActivationNeedsItsPlayerState(t *testing.T) {
	t.Parallel()

	t.Run("Threshold", func(t *testing.T) {
		t.Parallel()
		g, p, _ := newTwoPlayerGame(t)
		card := g.NewCard(creatureDefWithAbility(t, "Test Threshold",
			"AB$ GainLife | Cost$ 0 | Defined$ You | LifeAmount$ 1 | Activation$ Threshold"), p, engine.Battlefield)
		for i := 0; i < 6; i++ {
			g.NewCard(creatureDef(t), p, engine.Graveyard)
		}
		if g.ActivateAbility(p, card, 0, engine.NewScriptedController()) {
			t.Error("activated with six cards in the graveyard")
		}
		g.NewCard(creatureDef(t), p, engine.Graveyard)
		if !g.ActivateAbility(p, card, 0, engine.NewScriptedController()) {
			t.Error("declined with seven cards in the graveyard")
		}
	})
	t.Run("LifeTotal", func(t *testing.T) {
		t.Parallel()
		g, p, _ := newTwoPlayerGame(t)
		card := g.NewCard(creatureDefWithAbility(t, "Test Low Life",
			"AB$ GainLife | Cost$ 0 | Defined$ You | LifeAmount$ 1 | LifeTotal$ You | LifeAmount$ LE5"), p, engine.Battlefield)
		if g.ActivateAbility(p, card, 0, engine.NewScriptedController()) {
			t.Error("activated at 20 life")
		}
		g.Player(p).Life = 5
		if !g.ActivateAbility(p, card, 0, engine.NewScriptedController()) {
			t.Error("declined at 5 life")
		}
	})
	t.Run("CheckSVar", func(t *testing.T) {
		t.Parallel()
		g, p, _ := newTwoPlayerGame(t)
		off := g.NewCard(creatureDefWithAbility(t, "Test Check Off",
			"AB$ GainLife | Cost$ 0 | Defined$ You | LifeAmount$ 1 | CheckSVar$ 0 | SVarCompare$ GE1"), p, engine.Battlefield)
		on := g.NewCard(creatureDefWithAbility(t, "Test Check On",
			"AB$ GainLife | Cost$ 0 | Defined$ You | LifeAmount$ 1 | CheckSVar$ 2 | SVarCompare$ GE2"), p, engine.Battlefield)
		if g.ActivateAbility(p, off, 0, engine.NewScriptedController()) {
			t.Error("0 >= 1 activated")
		}
		if !g.ActivateAbility(p, on, 0, engine.NewScriptedController()) {
			t.Error("2 >= 2 declined")
		}
	})
}

// TestSpellNeedsItsPresentCondition proves a spell's own IsPresent$ refuses
// the cast with nothing paid.
func TestSpellNeedsItsPresentCondition(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGame(t)
	g.Player(p).ManaPool.Add(mana.White, 1)
	spell := g.NewCard(instantDefWithAbility(t, "Test Needs Creature", "W",
		"SP$ GainLife | Defined$ You | LifeAmount$ 3 | IsPresent$ Creature.YouCtrl"), p, engine.Hand)

	if g.CastSpell(p, spell, engine.NewScriptedController()) {
		t.Fatal("cast with no creature to satisfy IsPresent$")
	}
	if n := g.Player(p).ManaPool.Total(); n != 1 {
		t.Errorf("mana left = %d, want 1: a declined cast pays nothing", n)
	}
	g.NewCard(layer456Creature(t, "G"), p, engine.Battlefield)
	if !g.CastSpell(p, spell, engine.NewScriptedController()) {
		t.Error("cast declined with a creature in play")
	}
}
