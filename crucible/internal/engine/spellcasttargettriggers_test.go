package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
	"github.com/jczastkiewicz/crucible/internal/mana"
)

// The SpellCast tests on the spell's targets and payment
// (TriggerSpellAbilityCastOrCopy.java:125-203): each case casts one spell
// under a watcher whose trigger gains 1 life, and reads whether it fired.

// castUnderWatcher puts a watcher with the trigger line onto p's battlefield,
// runs setup to make the spell and queue its answers, casts it and resolves
// the stack, and returns the life p gained.
func castUnderWatcher(t *testing.T, trigger string, setup func(g *engine.Game, p engine.PlayerID, watcher engine.CardID, c *engine.ScriptedController) engine.CardID) int {
	t.Helper()
	g, p, _ := newTwoPlayerGameOn(t, scenarioDB(t))
	g.SetTurnState(1, p, engine.Main1)
	watcher := g.NewCard(combatCreatureDef(t, "Test Watcher", nil,
		[]string{"Mode$ SpellCast | ValidActivatingPlayer$ You | TriggerZones$ Battlefield | Execute$ TrigGain | " + trigger},
		"TrigGain", "DB$ GainLife | Defined$ You | LifeAmount$ 1"), p, engine.Battlefield)
	c := engine.NewScriptedController()
	spell := setup(g, p, watcher, c)
	before := g.Player(p).Life
	castThenResolve(t, g, p, spell, c)
	if err := g.TakePendingError(); err != nil {
		t.Fatalf("%s: %v", trigger, err)
	}
	return g.Player(p).Life - before
}

// growthAt makes a Giant Growth targeting target, paid with green.
func growthAt(t *testing.T, target func(g *engine.Game, p engine.PlayerID, watcher engine.CardID) engine.CardID) func(*engine.Game, engine.PlayerID, engine.CardID, *engine.ScriptedController) engine.CardID {
	return func(g *engine.Game, p engine.PlayerID, watcher engine.CardID, c *engine.ScriptedController) engine.CardID {
		tgt := target(g, p, watcher)
		g.Player(p).ManaPool.Add(mana.Green, 1)
		c.QueueTargets([]engine.EntityID{engine.CardEntity(tgt)})
		return g.NewCard(corpusCard(t, "Giant Growth"), p, engine.Hand)
	}
}

func TestSpellCastTargetsValidReadsTheSpellsTargets(t *testing.T) {
	t.Parallel()

	self := growthAt(t, func(_ *engine.Game, _ engine.PlayerID, w engine.CardID) engine.CardID { return w })
	other := growthAt(t, func(g *engine.Game, p engine.PlayerID, _ engine.CardID) engine.CardID {
		return g.NewCard(corpusCard(t, "Grizzly Bears"), p, engine.Battlefield)
	})
	if got := castUnderWatcher(t, "TargetsValid$ Card.Self", self); got != 1 {
		t.Errorf("targeting the watcher: gained %d, want 1", got)
	}
	if got := castUnderWatcher(t, "TargetsValid$ Card.Self", other); got != 0 {
		t.Errorf("targeting another creature: gained %d, want 0", got)
	}
}

func TestSpellCastIsSingleTargetNeedsOneTarget(t *testing.T) {
	t.Parallel()

	targeted := growthAt(t, func(_ *engine.Game, _ engine.PlayerID, w engine.CardID) engine.CardID { return w })
	untargeted := func(g *engine.Game, p engine.PlayerID, _ engine.CardID, _ *engine.ScriptedController) engine.CardID {
		return g.NewCard(corpusCard(t, "Ornithopter"), p, engine.Hand)
	}
	if got := castUnderWatcher(t, "IsSingleTarget$ True", targeted); got != 1 {
		t.Errorf("one target: gained %d, want 1", got)
	}
	if got := castUnderWatcher(t, "IsSingleTarget$ True", untargeted); got != 0 {
		t.Errorf("no target: gained %d, want 0", got)
	}
}

// CanTargetOtherCondition$ Creature.YouCtrl: the spell could have targeted
// another creature of yours.
func TestSpellCastCanTargetOtherConditionReadsTheOtherCandidates(t *testing.T) {
	t.Parallel()

	withOther := growthAt(t, func(g *engine.Game, p engine.PlayerID, _ engine.CardID) engine.CardID {
		return g.NewCard(corpusCard(t, "Grizzly Bears"), p, engine.Battlefield)
	})
	alone := growthAt(t, func(_ *engine.Game, _ engine.PlayerID, w engine.CardID) engine.CardID { return w })
	if got := castUnderWatcher(t, "CanTargetOtherCondition$ Creature.YouCtrl", withOther); got != 1 {
		t.Errorf("the watcher another candidate: gained %d, want 1", got)
	}
	if got := castUnderWatcher(t, "CanTargetOtherCondition$ Creature.YouCtrl+Other", alone); got != 0 {
		t.Errorf("no other candidate: gained %d, want 0", got)
	}
}

// NoColoredMana$ and ValidSAonCard$ Spell.ManaSpent read the cast's payment.
func TestSpellCastPaymentTests(t *testing.T) {
	t.Parallel()

	free := func(g *engine.Game, p engine.PlayerID, _ engine.CardID, _ *engine.ScriptedController) engine.CardID {
		return g.NewCard(corpusCard(t, "Ornithopter"), p, engine.Hand)
	}
	giant := func(g *engine.Game, p engine.PlayerID, _ engine.CardID, c *engine.ScriptedController) engine.CardID {
		g.Player(p).ManaPool.Add(mana.Red, 4)
		for range 3 {
			c.QueuePayGeneric(mana.ShardR)
		}
		return g.NewCard(corpusCard(t, "Hill Giant"), p, engine.Hand)
	}
	for _, tc := range []struct {
		trigger string
		setup   func(*engine.Game, engine.PlayerID, engine.CardID, *engine.ScriptedController) engine.CardID
		want    int
	}{
		{"NoColoredMana$ True", free, 1},
		{"NoColoredMana$ True", giant, 0},
		{"ValidSAonCard$ Spell.ManaSpent GE4", giant, 1},
		{"ValidSAonCard$ Spell.ManaSpent GE4", free, 0},
	} {
		if got := castUnderWatcher(t, tc.trigger, tc.setup); got != tc.want {
			t.Errorf("%s: gained %d, want %d", tc.trigger, got, tc.want)
		}
	}
}
