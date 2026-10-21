package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
)

// TestAddTurnEffectTakesExtraTurnThenResumes proves AddTurn: the next turn
// is the extra one, and turn order resumes normally after it.
func TestAddTurnEffectTakesExtraTurnThenResumes(t *testing.T) {
	t.Parallel()

	g, p, other := newTwoPlayerGame(t)
	c := engine.NewScriptedController()
	if _, err := castETBChain(t, g, p, etbChainDef(t, "Test Time Walk", "DB$ AddTurn | Defined$ You | NumTurns$ 1"), c); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	g.SetTurnState(1, p, engine.Cleanup)
	g.AdvancePhase(c)
	if got := g.ActivePlayer(); got != p {
		t.Fatalf("turn 2 active = %v, want %v (extra turn)", got, p)
	}
	g.SetTurnState(2, p, engine.Cleanup)
	g.AdvancePhase(c)
	if got := g.ActivePlayer(); got != other {
		t.Errorf("turn 3 active = %v, want %v", got, other)
	}
}

// TestSkipTurnEffectSkipsNextTurn proves SkipTurn: the skipped player's
// turn is passed over, once.
func TestSkipTurnEffectSkipsNextTurn(t *testing.T) {
	t.Parallel()

	g, p, other := newTwoPlayerGame(t)
	c := engine.NewScriptedController()
	if _, err := castETBChain(t, g, p, etbChainDef(t, "Test Skip", "DB$ SkipTurn | Defined$ Opponent | NumTurns$ 1"), c); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	g.SetTurnState(1, p, engine.Cleanup)
	g.AdvancePhase(c)
	if got := g.ActivePlayer(); got != p {
		t.Fatalf("next active = %v, want %v (opponent skipped)", got, p)
	}
	g.SetTurnState(2, p, engine.Cleanup)
	g.AdvancePhase(c)
	if got := g.ActivePlayer(); got != other {
		t.Errorf("following active = %v, want %v", got, other)
	}
}

// TestAddTurnEffectRejectsUnresolvedParam proves SkipUntap$ fails closed.
func TestAddTurnEffectRejectsUnresolvedParam(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGame(t)
	def := etbChainDef(t, "Test Time Walk Bad", "DB$ AddTurn | Defined$ You | NumTurns$ 1 | SkipUntap$ True")
	if _, err := castETBChain(t, g, p, def, engine.NewScriptedController()); err == nil {
		t.Fatal("ResolveStack succeeded, want an error for unresolved SkipUntap$")
	}
}

// TestAddTurnEffectDelayedTriggerFiresOnTheExtraTurnOnly proves
// ExtraTurnDelayedTrigger$ (Warrior's Oath): the end step of the turn the
// spell resolves in is untouched, the end step of the extra turn makes the
// caster lose, and the trigger is gone afterwards.
func TestAddTurnEffectDelayedTriggerFiresOnTheExtraTurnOnly(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGame(t)
	c := engine.NewScriptedController()
	if _, err := resolveNow(t, g, p, c, nil,
		"DB$ AddTurn | NumTurns$ 1 | ExtraTurnDelayedTrigger$ DBDelTrig | ExtraTurnDelayedTriggerExecute$ TrigLose",
		"DBDelTrig", "ThisTurn$ True | Mode$ Phase | Phase$ End of Turn | Execute$ TrigLose",
		"TrigLose", "DB$ LosesGame | Defined$ You"); err != nil {
		t.Fatal(err)
	}
	advanceToPhase(t, g, c, 1, engine.EndOfTurn)
	if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
		t.Fatal(err)
	}
	if g.Player(p).Lost {
		t.Fatal("caster lost at the end step of the turn the spell resolved in")
	}
	advanceToPhase(t, g, c, 2, engine.EndOfTurn)
	if g.ActivePlayer() != p {
		t.Fatalf("turn 2 active = %v, want the extra turn's player %v", g.ActivePlayer(), p)
	}
	if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
		t.Fatal(err)
	}
	if !g.Player(p).Lost {
		t.Error("caster did not lose at the end step of the extra turn")
	}
}

// TestAddTurnEffectDelayedTriggerRejectsOtherModes proves a mode the
// delayed registry cannot fire fails closed.
func TestAddTurnEffectDelayedTriggerRejectsOtherModes(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGame(t)
	_, err := resolveNow(t, g, p, engine.NewScriptedController(), nil,
		"DB$ AddTurn | NumTurns$ 1 | ExtraTurnDelayedTrigger$ DBDelTrig | ExtraTurnDelayedTriggerExecute$ TrigLose",
		"DBDelTrig", "ThisTurn$ True | Mode$ Drawn | Execute$ TrigLose",
		"TrigLose", "DB$ LosesGame | Defined$ You")
	if err == nil {
		t.Fatal("ResolveStack succeeded, want an error for an unresolved trigger Mode$")
	}
}

// ChangeZone Transformed$ (ChangeZoneEffect.java:598): the card turns to its
// back face before it enters, so it arrives as the back face.
func TestChangeZoneTransformedEntersOnItsBackFace(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGameOn(t, scenarioDB(t))
	card := g.NewCard(corpusCard(t, "Startled Awake"), p, engine.Graveyard)
	if _, err := resolveNow(t, g, p, engine.NewScriptedController(), []engine.EntityID{engine.CardEntity(card)},
		"DB$ ChangeZone | Origin$ Graveyard | Destination$ Battlefield | ValidTgts$ Card | Transformed$ True"); err != nil {
		t.Fatal(err)
	}
	got := g.Card(card)
	if got.Zone != engine.Battlefield || got.Def.Name != "Persistent Nightmare" {
		t.Errorf("card is %q in %v, want Persistent Nightmare on the battlefield", got.Def.Name, got.Zone)
	}
}
