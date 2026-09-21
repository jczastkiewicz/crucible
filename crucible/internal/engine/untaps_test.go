package engine_test

import (
	"strings"
	"testing"

	"github.com/jczastkiewicz/crucible/internal/carddb"
	"github.com/jczastkiewicz/crucible/internal/carddb/compile"
	"github.com/jczastkiewicz/crucible/internal/cardtype"
	"github.com/jczastkiewicz/crucible/internal/engine"
)

// untapsCreatureDef builds a *compile.Card for a creature carrying a real
// "whenever CARDNAME becomes untapped" trigger (Inspired's own real corpus
// shape, the dominant one among 30 real Mode$ Untaps lines) that gains its
// controller 5 life -- an inert, always-succeeding Execute$ so a test can
// tell "the trigger fired" apart from "the effect it named happened to also
// work" (becomestarget_test.go's own identical reasoning).
func untapsCreatureDef(t *testing.T, name, validCard, extraTriggerParams string) *compile.Card {
	t.Helper()

	reg, err := cardtype.LoadRegistry(strings.NewReader("[CreatureTypes]\nElf\n"))
	if err != nil {
		t.Fatalf("LoadRegistry: %v", err)
	}
	raw := &carddb.Card{Filename: name}
	raw.Faces[0].Present = true
	raw.Faces[0].Name = name
	raw.Faces[0].Type = cardtype.Parse(reg, "Creature Elf")
	raw.Faces[0].Power, raw.Faces[0].Toughness = "1", "1"
	trigger := "Mode$ Untaps | ValidCard$ " + validCard
	if extraTriggerParams != "" {
		trigger += " | " + extraTriggerParams
	}
	trigger += " | Execute$ TrigGainLife"
	raw.Faces[0].Triggers = []string{trigger}
	raw.Faces[0].SVars.Set("TrigGainLife", "DB$ GainLife | Defined$ You | LifeAmount$ 5")

	c, err := compile.Compile(raw)
	if err != nil {
		t.Fatalf("compile %q: %v", name, err)
	}
	return c
}

// TestStartTurnFiresUntapsTriggerForSelf proves checkUntapsTriggers
// (trigger.go) is wired into untapStep (turn.go): a creature's own
// "Inspired" trigger (ValidCard$ Card.Self) fires when it untaps at the
// start of its controller's turn.
func TestStartTurnFiresUntapsTriggerForSelf(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	a, b := g.Players()[0], g.Players()[1]
	g.Player(a).Life, g.Player(b).Life = 20, 20
	c := g.NewCard(untapsCreatureDef(t, "Test Inspired", "Card.Self", ""), a, engine.Battlefield)
	g.Card(c).Tapped = true
	sc := engine.NewScriptedController()

	g.StartTurn(a, sc)
	if err := g.ResolveStack(engine.NewRegistry(), sc); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}

	if g.Card(c).Tapped {
		t.Fatal("card is still tapped after its controller's own untap step")
	}
	if got := g.Player(a).Life; got != 25 {
		t.Errorf("a life = %d, want 25 -- the untapping card's own Inspired trigger should have fired", got)
	}
}

// TestStartTurnFiresUntapsTriggerForOtherPermanent proves
// mesmeric_orb.txt's own real bare-Card shape: a separate watcher's own
// "whenever a permanent becomes untapped" fires for some OTHER permanent
// untapping, the identical single-walk shape checkTapsTriggers already has
// for its own mirror event.
func TestStartTurnFiresUntapsTriggerForOtherPermanent(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	a, b := g.Players()[0], g.Players()[1]
	g.Player(a).Life, g.Player(b).Life = 20, 20
	watcher := g.NewCard(untapsCreatureDef(t, "Test Watcher", "Card", ""), a, engine.Battlefield)
	g.Card(watcher).Tapped = false
	other := g.NewCard(nil, a, engine.Battlefield)
	g.Card(other).Tapped = true
	sc := engine.NewScriptedController()

	g.StartTurn(a, sc)
	if err := g.ResolveStack(engine.NewRegistry(), sc); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}

	if got := g.Player(a).Life; got != 25 {
		t.Errorf("a life = %d, want 25 -- the watcher's own bare Card$ trigger should have fired for other untapping", got)
	}
}

// TestStartTurnSkipsUntapsTriggerForAlreadyUntappedCard proves the wasTapped
// guard (untapStep, turn.go): a card that was never tapped generates no
// untapping event at all, matching Card.untap()'s own early
// "if (!tapped) return false" before the trigger even fires.
func TestStartTurnSkipsUntapsTriggerForAlreadyUntappedCard(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	a, b := g.Players()[0], g.Players()[1]
	g.Player(a).Life, g.Player(b).Life = 20, 20
	c := g.NewCard(untapsCreatureDef(t, "Test Inspired", "Card.Self", ""), a, engine.Battlefield)
	g.Card(c).Tapped = false

	g.StartTurn(a, engine.NewScriptedController())

	if got := g.StackLen(); got != 0 {
		t.Fatalf("StackLen() = %d, want 0 -- a card already untapped must never push its own Untaps trigger", got)
	}
	if got := g.Player(a).Life; got != 20 {
		t.Errorf("a life = %d, want unchanged 20 -- a card already untapped must not fire its own Untaps trigger", got)
	}
}

// TestStartTurnFiresUntapsTriggerNamingOptionalDeciderWhenConfirmed proves
// OptionalDecider$ You (3 of 30 real Mode$ Untaps lines) resolves through
// triggerEffectAPI's own triggerIsOptional (trigger.go): confirmed, the
// ability runs exactly as if it had not been optional at all.
func TestStartTurnFiresUntapsTriggerNamingOptionalDeciderWhenConfirmed(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	a, b := g.Players()[0], g.Players()[1]
	g.Player(a).Life, g.Player(b).Life = 20, 20
	c := g.NewCard(untapsCreatureDef(t, "Test Inspired", "Card.Self", "OptionalDecider$ You"), a, engine.Battlefield)
	g.Card(c).Tapped = true
	sc := engine.NewScriptedController()
	sc.QueueConfirmOptionalTrigger(true)

	g.StartTurn(a, sc)
	if err := g.ResolveStack(engine.NewRegistry(), sc); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}

	if got := g.Player(a).Life; got != 25 {
		t.Errorf("a life = %d, want 25 -- a confirmed OptionalDecider$ You trigger must run its own body", got)
	}
}

// TestStartTurnSkipsUntapsTriggerNamingOptionalDeciderWhenDeclined is the
// same trigger's own negative twin: declined, the ability does nothing at
// all, the identical outcome an unmet mandatory restriction already has.
func TestStartTurnSkipsUntapsTriggerNamingOptionalDeciderWhenDeclined(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	a, b := g.Players()[0], g.Players()[1]
	g.Player(a).Life, g.Player(b).Life = 20, 20
	c := g.NewCard(untapsCreatureDef(t, "Test Inspired", "Card.Self", "OptionalDecider$ You"), a, engine.Battlefield)
	g.Card(c).Tapped = true
	sc := engine.NewScriptedController()
	sc.QueueConfirmOptionalTrigger(false)

	g.StartTurn(a, sc)
	if err := g.ResolveStack(engine.NewRegistry(), sc); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}

	if got := g.Player(a).Life; got != 20 {
		t.Errorf("a life = %d, want unchanged 20 -- a declined OptionalDecider$ You trigger must not run its own body", got)
	}
}

// TestStartTurnSkipsUntapsTriggerNamingUnresolvedOptionalDecider proves the
// PORT-8 skip for an OptionalDecider$ value other than "You": the trigger
// never even reaches the confirm step, since this port has no resolver for
// who TriggeredCardController names as the decider (triggerIsOptional's own
// doc comment) -- the identical "skip the whole line, don't guess" contract
// every other unresolved trigger restriction in this file already has.
func TestStartTurnSkipsUntapsTriggerNamingUnresolvedOptionalDecider(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	a, b := g.Players()[0], g.Players()[1]
	g.Player(a).Life, g.Player(b).Life = 20, 20
	c := g.NewCard(untapsCreatureDef(t, "Test Inspired", "Card.Self", "OptionalDecider$ TriggeredCardController"), a, engine.Battlefield)
	g.Card(c).Tapped = true
	sc := engine.NewScriptedController()

	g.StartTurn(a, sc)
	if err := g.ResolveStack(engine.NewRegistry(), sc); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}

	if got := g.Player(a).Life; got != 20 {
		t.Errorf("a life = %d, want unchanged 20 -- an unresolved OptionalDecider$ value must skip the whole line, never asking", got)
	}
}
