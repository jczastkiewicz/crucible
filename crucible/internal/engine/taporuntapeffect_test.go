package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
)

// TestTapOrUntapEffectChoosesTap proves the dominant shape (38 of 41 real
// lines name ValidTgts$): the activator decides, and "tap" taps an untapped
// target.
func TestTapOrUntapEffectChoosesTap(t *testing.T) {
	t.Parallel()

	g, p, other := newTwoPlayerGame(t)
	target := g.NewCard(creatureDefPT(t, "2", "2"), other, engine.Battlefield)

	c := engine.NewScriptedController()
	c.QueueTargets([]engine.EntityID{engine.CardEntity(target)})
	c.QueueTapOrUntap(true)
	if _, err := castETBChain(t, g, p, etbChainDef(t, "Test TapOrUntap", "DB$ TapOrUntap | ValidTgts$ Creature"), c); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if !g.Card(target).Tapped {
		t.Error("target Tapped = false, want true")
	}
}

// TestTapOrUntapEffectChoosesUntap proves the other branch: "untap" untaps
// a tapped target.
func TestTapOrUntapEffectChoosesUntap(t *testing.T) {
	t.Parallel()

	g, p, other := newTwoPlayerGame(t)
	target := g.NewCard(creatureDefPT(t, "2", "2"), other, engine.Battlefield)
	g.Card(target).Tapped = true

	c := engine.NewScriptedController()
	c.QueueTargets([]engine.EntityID{engine.CardEntity(target)})
	c.QueueTapOrUntap(false)
	if _, err := castETBChain(t, g, p, etbChainDef(t, "Test TapOrUntap Untap", "DB$ TapOrUntap | ValidTgts$ Creature"), c); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if g.Card(target).Tapped {
		t.Error("target Tapped = true, want false")
	}
}

// TestTapOrUntapEffectToggleSkipsDecision proves Toggle$ flips the state
// without asking the controller (no answer queued).
func TestTapOrUntapEffectToggleSkipsDecision(t *testing.T) {
	t.Parallel()

	g, p, other := newTwoPlayerGame(t)
	target := g.NewCard(creatureDefPT(t, "2", "2"), other, engine.Battlefield)
	g.Card(target).Tapped = true

	c := engine.NewScriptedController()
	c.QueueTargets([]engine.EntityID{engine.CardEntity(target)})
	if _, err := castETBChain(t, g, p, etbChainDef(t, "Test TapOrUntap Toggle", "DB$ TapOrUntap | ValidTgts$ Creature | Toggle$ True"), c); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if g.Card(target).Tapped {
		t.Error("target Tapped = true, want false after Toggle$")
	}
}

// TestTapOrUntapEffectRejectsUnresolvedParam proves Tapper$ fails closed.
func TestTapOrUntapEffectRejectsUnresolvedParam(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGame(t)
	c := engine.NewScriptedController()
	if _, err := castETBChain(t, g, p, etbChainDef(t, "Test TapOrUntap Tapper", "DB$ TapOrUntap | Defined$ Self | Tapper$ Opponent"), c); err == nil {
		t.Fatal("ResolveStack succeeded, want an error for unresolved Tapper$")
	}
}

// TestTapOrUntapEffectSkipsCardOffBattlefield proves a Defined$ card that
// is not on the battlefield is skipped without asking.
func TestTapOrUntapEffectSkipsCardOffBattlefield(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGame(t)
	c := engine.NewScriptedController()
	def := etbChainDef(t, "Test TapOrUntap Offfield", "DB$ ChooseCard | ChoiceZone$ Hand | Choices$ Card | Mandatory$ True | SubAbility$ DBTou",
		"DBTou", "DB$ TapOrUntap | Defined$ ChosenCard")
	inHand := g.NewCard(creatureDefPT(t, "2", "2"), p, engine.Hand)
	c.QueueCardChoice([]engine.CardID{inHand})
	if _, err := castETBChain(t, g, p, def, c); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if g.Card(inHand).Tapped {
		t.Error("hand card Tapped = true, want untouched")
	}
}
