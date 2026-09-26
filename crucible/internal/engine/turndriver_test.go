package engine_test

import (
	"errors"
	"testing"

	"github.com/jczastkiewicz/crucible/internal/carddb"
	"github.com/jczastkiewicz/crucible/internal/carddb/compile"
	"github.com/jczastkiewicz/crucible/internal/cardtype"
	"github.com/jczastkiewicz/crucible/internal/engine"
	"github.com/jczastkiewicz/crucible/internal/mana"
)

// discardPunisherDef is Megrim's own trigger on a test enchantment:
// whenever an opponent discards a card, 2 damage to that player.
func discardPunisherDef(t *testing.T) *compile.Card {
	t.Helper()

	raw := &carddb.Card{Filename: "Test Punisher"}
	raw.Faces[0].Present = true
	raw.Faces[0].Name = "Test Punisher"
	raw.Faces[0].Type = cardtype.Parse(attachmentTypeRegistry(t), "Enchantment")
	raw.Faces[0].Triggers = []string{"Mode$ Discarded | ValidCard$ Card.OppOwn | TriggerZones$ Battlefield | Execute$ TrigDealDamage"}
	raw.Faces[0].SVars.Set("TrigDealDamage", "DB$ DealDamage | Defined$ TriggeredCardController | NumDmg$ 2")

	c, err := compile.Compile(raw)
	if err != nil {
		t.Fatalf("compile Test Punisher: %v", err)
	}
	return c
}

// TestStepRepeatsCleanupWhenItGrantsPriority is CR 514.3a (ADR-0026
// Decision 1): discarding to hand size in the cleanup step triggers the
// opponent's Megrim-shaped enchantment, the non-empty stack grants priority, the trigger
// resolves, and a second cleanup step begins -- one that finds nothing to
// do and ends the call. The second PhaseBegan(Cleanup) is the repeat; no
// state a fixture's expect.state can see tells the two apart, which is why
// this is a Go test and not a scenario.
func TestStepRepeatsCleanupWhenItGrantsPriority(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	a, b := g.Players()[0], g.Players()[1]
	g.SetTurnState(1, a, engine.EndOfTurn)
	g.Player(a).Life, g.Player(b).Life = 20, 20
	var sink recordingSink
	g.SetSink(&sink)

	g.NewCard(discardPunisherDef(t), b, engine.Battlefield)
	var hand []engine.CardID
	for i := 0; i < engine.MaxHandSize+1; i++ {
		hand = append(hand, g.NewCard(creatureDef(t), a, engine.Hand))
	}

	c := engine.NewScriptedController()
	c.QueueDiscard(hand[:1])

	if err := g.Step(engine.NewRegistry(), c); err != nil {
		t.Fatalf("Step: %v", err)
	}

	cleanups := 0
	for _, e := range sink.events {
		if e.Kind == engine.PhaseBegan && e.Phase == engine.Cleanup {
			cleanups++
		}
	}
	if cleanups != 2 {
		t.Errorf("cleanup steps begun = %d, want 2 (CR 514.3a repeat)", cleanups)
	}
	if got := g.Player(a).Life; got != 18 {
		t.Errorf("a life = %d, want 18 (the discard trigger resolved in cleanup)", got)
	}
	if g.ActivePhase() != engine.Cleanup {
		t.Errorf("ActivePhase() = %v, want Cleanup", g.ActivePhase())
	}
	if g.StackLen() != 0 {
		t.Errorf("StackLen() = %d, want 0", g.StackLen())
	}
}

// TestStepRepeatsCleanupWhenAStateBasedActionIsPerformed is CR 514.3a's
// other half: nothing is on the stack, but the cleanup step's own
// state-based-action check puts a 0-toughness creature into the graveyard
// (CR 704.5f), so priority is granted and another cleanup step follows.
// Nothing checks state-based actions between setup and the cleanup step's
// body, so that check is the one that performs the action.
func TestStepRepeatsCleanupWhenAStateBasedActionIsPerformed(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	a, b := g.Players()[0], g.Players()[1]
	g.SetTurnState(1, a, engine.EndOfTurn)
	g.Player(a).Life, g.Player(b).Life = 20, 20
	var sink recordingSink
	g.SetSink(&sink)
	doomed := g.NewCard(creatureDefPT(t, "1", "0"), a, engine.Battlefield)

	if err := g.Step(engine.NewRegistry(), engine.NewScriptedController()); err != nil {
		t.Fatalf("Step: %v", err)
	}
	cleanups := 0
	for _, e := range sink.events {
		if e.Kind == engine.PhaseBegan && e.Phase == engine.Cleanup {
			cleanups++
		}
	}
	if cleanups != 2 {
		t.Errorf("cleanup steps begun = %d, want 2 (CR 514.3a repeat after an SBA)", cleanups)
	}
	if got := g.Card(doomed).Zone; got != engine.Graveyard {
		t.Errorf("0-toughness creature zone = %v, want Graveyard", got)
	}
}

// TestStepEndsCleanupWithoutPriorityWhenNothingHappens is CR 514.3's
// ordinary case: a cleanup step with nothing to discard and no trigger
// grants no priority and begins once.
func TestStepEndsCleanupWithoutPriorityWhenNothingHappens(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	a, b := g.Players()[0], g.Players()[1]
	g.SetTurnState(1, a, engine.EndOfTurn)
	g.Player(a).Life, g.Player(b).Life = 20, 20
	var sink recordingSink
	g.SetSink(&sink)

	if err := g.Step(engine.NewRegistry(), engine.NewScriptedController()); err != nil {
		t.Fatalf("Step: %v", err)
	}
	cleanups := 0
	for _, e := range sink.events {
		if e.Kind == engine.PhaseBegan && e.Phase == engine.Cleanup {
			cleanups++
		}
	}
	if cleanups != 1 {
		t.Errorf("cleanup steps begun = %d, want 1", cleanups)
	}
}

// TestRunBeforeStartTurnErrors is GO-7 at the driver's own entry: a game
// with no turn begun has no step to leave.
func TestRunBeforeStartTurnErrors(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	if err := g.Run(engine.NewRegistry(), engine.NewScriptedController(), 1); err == nil {
		t.Fatal("Run before StartTurn: err = nil, want an error")
	}
}

// TestPassPriorityErrorsOnMultiColorTapForMana: a controller's
// ActionTapForMana naming more than one color is that controller's error,
// not TapLandForMana's invariant panic (GO-7, ADR-0026 Decision 4).
func TestPassPriorityErrorsOnMultiColorTapForMana(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	a, b := g.Players()[0], g.Players()[1]
	g.SetTurnState(1, a, engine.Main1)
	g.Player(a).Life, g.Player(b).Life = 20, 20
	land := g.NewCard(creatureDef(t), a, engine.Battlefield)

	c := engine.NewScriptedController()
	c.QueueAction(a, engine.Action{Kind: engine.ActionTapForMana, Card: land, Color: mana.Red | mana.Green})

	err := g.PassPriority(engine.NewRegistry(), c)
	if err == nil {
		t.Fatal("PassPriority: err = nil, want an error")
	}
	var panicked interface{ RuntimeError() }
	if errors.As(err, &panicked) {
		t.Fatalf("PassPriority: runtime error %v, want a controller error", err)
	}
}
