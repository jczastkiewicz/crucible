package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
)

// TestGainControlEffectTakesAndUntaps proves GainControl's permanent shape:
// the activator controls the target, it is untapped (Untap$) and summoning
// sick, and control returns to the owner once it leaves the battlefield.
func TestGainControlEffectTakesAndUntaps(t *testing.T) {
	t.Parallel()

	g, p, other := newTwoPlayerGame(t)
	target := g.NewCard(creatureDefPT(t, "2", "2"), other, engine.Battlefield)
	g.Card(target).Tapped = true

	c := engine.NewScriptedController()
	c.QueueTargets([]engine.EntityID{engine.CardEntity(target)})
	def := etbChainDef(t, "Test Steal", "DB$ GainControl | ValidTgts$ Creature.OppCtrl | Untap$ True")
	if _, err := castETBChain(t, g, p, def, c); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	card := g.Card(target)
	if card.Controller() != p || card.Tapped || !card.SummonSick {
		t.Errorf("controller %v tapped %v sick %v, want %v false true", card.Controller(), card.Tapped, card.SummonSick, p)
	}
	g.Move(target, engine.Graveyard, other)
	if got := g.Card(target).Controller(); got != other {
		t.Errorf("controller after leaving = %v, want owner %v", got, other)
	}
}

// TestExchangeControlEffectSwaps proves ExchangeControl: two targets trade
// controllers.
func TestExchangeControlEffectSwaps(t *testing.T) {
	t.Parallel()

	g, p, other := newTwoPlayerGame(t)
	mine := g.NewCard(creatureDefPT(t, "1", "1"), p, engine.Battlefield)
	theirs := g.NewCard(creatureDefPT(t, "5", "5"), other, engine.Battlefield)

	c := engine.NewScriptedController()
	c.QueueTargets([]engine.EntityID{engine.CardEntity(mine), engine.CardEntity(theirs)})
	def := etbChainDef(t, "Test Switch", "DB$ ExchangeControl | ValidTgts$ Creature | TargetMin$ 2 | TargetMax$ 2 | RememberExchanged$ True")
	host, err := castETBChain(t, g, p, def, c)
	if err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if g.Card(mine).Controller() != other || g.Card(theirs).Controller() != p {
		t.Errorf("controllers = %v/%v, want %v/%v", g.Card(mine).Controller(), g.Card(theirs).Controller(), other, p)
	}
	if n := len(g.Card(host).Memory.Remembered()); n != 2 {
		t.Errorf("remembered = %d, want 2", n)
	}
}

// TestGainControlEffectRejectsUnresolvedParam proves LoseControl$ fails closed.
func TestGainControlEffectRejectsUnresolvedParam(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGame(t)
	def := etbChainDef(t, "Test Threaten", "DB$ GainControl | Defined$ Self | LoseControl$ EOT")
	if _, err := castETBChain(t, g, p, def, engine.NewScriptedController()); err == nil {
		t.Fatal("ResolveStack succeeded, want an error for unresolved LoseControl$")
	}
}

// TestGainControlEffectSecondaryShapes proves NewController$, Optional$
// declined, RememberControlled$/ForgetControlled$, and ExchangeControl's
// Defined$ reading and off-battlefield no-op.
func TestGainControlEffectSecondaryShapes(t *testing.T) {
	t.Parallel()

	t.Run("NewControllerRemember", func(t *testing.T) {
		g, p, other := newTwoPlayerGame(t)
		c := engine.NewScriptedController()
		host, err := castETBChain(t, g, p, etbChainDef(t, "Test Donate", "DB$ GainControl | Defined$ Self | NewController$ Opponent | RememberControlled$ True"), c)
		if err != nil {
			t.Fatalf("ResolveStack: %v", err)
		}
		if got := g.Card(host).Controller(); got != other {
			t.Errorf("controller = %v, want %v", got, other)
		}
		if n := len(g.Card(host).Memory.Remembered()); n != 1 {
			t.Errorf("remembered = %d, want 1", n)
		}
	})
	t.Run("OptionalDeclineForget", func(t *testing.T) {
		g, p, other := newTwoPlayerGame(t)
		c := engine.NewScriptedController()
		c.QueueConfirmEffect(false)
		host, err := castETBChain(t, g, p, etbChainDef(t, "Test Donate Opt", "DB$ GainControl | Defined$ Self | NewController$ Opponent | Optional$ True | ForgetControlled$ True"), c)
		if err != nil {
			t.Fatalf("ResolveStack: %v", err)
		}
		if got := g.Card(host).Controller(); got == other {
			t.Errorf("controller = %v, want unchanged", got)
		}
	})
	t.Run("ExchangeDefined", func(t *testing.T) {
		g, p, other := newTwoPlayerGame(t)
		theirs := g.NewCard(creatureDefPT(t, "2", "2"), other, engine.Battlefield)
		c := engine.NewScriptedController()
		c.QueueTargets([]engine.EntityID{engine.CardEntity(theirs)})
		c.QueueConfirmEffect(true)
		host, err := castETBChain(t, g, p, etbChainDef(t, "Test Exchange Self", "DB$ ExchangeControl | ValidTgts$ Creature.OppCtrl | Defined$ Self | Optional$ True"), c)
		if err != nil {
			t.Fatalf("ResolveStack: %v", err)
		}
		if g.Card(host).Controller() != other || g.Card(theirs).Controller() != p {
			t.Error("Defined$ exchange did not swap")
		}
	})
	t.Run("ExchangeMissingObject", func(t *testing.T) {
		g, p, _ := newTwoPlayerGame(t)
		host, err := castETBChain(t, g, p, etbChainDef(t, "Test Exchange Missing", "DB$ ExchangeControl | Defined$ Self"), engine.NewScriptedController())
		if err != nil {
			t.Fatalf("ResolveStack: %v", err)
		}
		if got := g.Card(host).Controller(); got != p {
			t.Errorf("controller = %v, want unchanged", got)
		}
	})
}
