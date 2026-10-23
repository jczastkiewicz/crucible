package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
)

// Mode$ Always (TriggerAlways.java, GameAction.java:1244): a state trigger
// fires as soon as its IsPresent$ or LifeTotal$ condition holds after a
// state-based action check, and not again while it waits on the stack.
func TestAlwaysTriggerFiresWhileItsConditionHolds(t *testing.T) {
	t.Parallel()

	const trigger = "T:Mode$ Always | TriggerZones$ Battlefield | IsPresent$ Island.YouCtrl | PresentCompare$ EQ0 | Execute$ TrigSac | TriggerDescription$ x"
	sac := "SVar:TrigSac:DB$ Sacrifice | SacValid$ Self"

	t.Run("no island", func(t *testing.T) {
		t.Parallel()

		g, p, _ := newTwoPlayerGame(t)
		card := g.NewCard(scriptDef(t, "Test Serpent", "Creature Serpent", trigger, sac), p, engine.Battlefield)
		c := engine.NewScriptedController()
		engine.CheckStateBasedActions(g, c)
		if got := g.StackLen(); got != 1 {
			t.Fatalf("stack = %d, want the one state trigger", got)
		}
		engine.CheckStateBasedActions(g, c)
		if got := g.StackLen(); got != 1 {
			t.Errorf("stack = %d after a second check, want the trigger not repeated", got)
		}
		if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
			t.Fatal(err)
		}
		if got := g.Card(card).Zone; got != engine.Graveyard {
			t.Errorf("serpent in %v, want the graveyard", got)
		}
	})
	t.Run("island present", func(t *testing.T) {
		t.Parallel()

		g, p, _ := newTwoPlayerGame(t)
		card := g.NewCard(scriptDef(t, "Test Serpent", "Creature Serpent", trigger, sac), p, engine.Battlefield)
		g.NewCard(scriptDef(t, "Test Island", "Land Island"), p, engine.Battlefield)
		c := engine.NewScriptedController()
		engine.CheckStateBasedActions(g, c)
		if got := g.StackLen(); got != 0 {
			t.Errorf("stack = %d, want no trigger while an Island is out", got)
		}
		if got := g.Card(card).Zone; got != engine.Battlefield {
			t.Errorf("serpent in %v, want the battlefield", got)
		}
	})
}

// ResolvingCheck$ re-tests one condition as the ability resolves
// (WrappedAbility.java:421-430): Opal Avenger's "if it is an enchantment".
func TestAlwaysTriggerResolvingCheck(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGame(t)
	g.NewCard(scriptDef(t, "Test Avenger", "Enchantment",
		"T:Mode$ Always | LifeTotal$ You | LifeAmount$ LE10 | TriggerZones$ Battlefield | Execute$ TrigGain | IsPresent$ Card.Self+Enchantment | ResolvingCheck$ IsPresent | TriggerDescription$ x",
		"SVar:TrigGain:DB$ GainLife | Defined$ You | LifeAmount$ 1"), p, engine.Battlefield)
	g.Player(p).Life = 5
	c := engine.NewScriptedController()
	engine.CheckStateBasedActions(g, c)
	if got := g.StackLen(); got != 1 {
		t.Fatalf("stack = %d, want the one state trigger", got)
	}
	if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
		t.Fatal(err)
	}
	// The trigger resolved (+1), then fired again while life stays at 10 or
	// less, until the loop reaches 11.
	if got := g.Player(p).Life; got != 11 {
		t.Errorf("life = %d, want 11", got)
	}
}
