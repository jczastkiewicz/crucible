package fixture_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
)

// `queue action` lines that name no well-formed action fail at parse time,
// before anything is queued.
func TestRunActionsQueueActionMalformedErrors(t *testing.T) {
	t.Parallel()

	for _, line := range []string{
		"queue action human",
		"queue action nobody pass",
		"queue action nobody cast 1",
		"queue action human cast abc",
		"queue action human cast 1,1",
		"queue action human cast 1 2",
		"queue action human playland",
		"queue action human activate 1",
		"queue action human activate 1 x",
		"queue action human manaability 1",
		"queue action human tapformana 1",
		"queue action human tapformana 1 Q",
		"queue action human sacrifice 1",
	} {
		t.Run(line, func(t *testing.T) {
			t.Parallel()

			db := landDB(t, "Mountain", "Basic Land Mountain")
			l := load(t, db, "humanlife=20\nailife=20\nhumanhand=Mountain|Id:1\n")
			if err := runActions(t, l, engine.NewScriptedController(), line+"\n"); err == nil {
				t.Errorf("%q did not error", line)
			}
		})
	}
}

// passpriority applies queued special actions and mana abilities: human
// plays a Mountain and taps it for R, keeping priority after each, then
// passes explicitly.
func TestRunActionsPassPriorityPlaysLandAndTapsForMana(t *testing.T) {
	t.Parallel()

	db := landDB(t, "Mountain", "Basic Land Mountain")
	l := load(t, db, "humanlife=20\nailife=20\nhumanhand=Mountain|Id:1\n")
	c := engine.NewScriptedController()

	log := "startturn human\nadvance 3\n" +
		"queue action human playland 1\n" +
		"queue action human tapformana 1 R\n" +
		"queue action human pass\n" +
		"passpriority\n"
	if err := runActions(t, l, c, log); err != nil {
		t.Fatalf("RunActions: %v", err)
	}
	mountain := l.CardByFixtureID[1]
	if got := l.Game.Card(mountain).Zone; got != engine.Battlefield {
		t.Errorf("Mountain zone %v, want Battlefield", got)
	}
	if !l.Game.Card(mountain).Tapped {
		t.Error("Mountain untapped, want tapped for mana")
	}
}

// A queued action the rules decline stops the round with an error naming
// it (ADR-0019 Decision point 5): a Mountain has no scripted mana ability.
func TestRunActionsPassPriorityDeclinedActionErrors(t *testing.T) {
	t.Parallel()

	db := landDB(t, "Mountain", "Basic Land Mountain")
	l := load(t, db, "humanlife=20\nailife=20\nhumanbattlefield=Mountain|Id:1\n")
	c := engine.NewScriptedController()

	log := "startturn human\nadvance 3\nqueue action human manaability 1 0\npasspriority\n"
	if err := runActions(t, l, c, log); err == nil {
		t.Error("declined mana ability did not error")
	}
}

// step drives one step at a time; run stops after the capped turn's
// cleanup step.
func TestRunActionsStepAndRun(t *testing.T) {
	t.Parallel()

	db := testDB(t)
	l := load(t, db, "humanlife=20\nailife=20\n")
	c := engine.NewScriptedController()

	// Upkeep, then Main1: turn 1 skips the draw step (CR 103.7a).
	if err := runActions(t, l, c, "startturn human\nstep\nstep 1\n"); err != nil {
		t.Fatalf("RunActions: %v", err)
	}
	if l.Game.ActivePhase() != engine.Main1 {
		t.Errorf("phase %v after two steps, want Main1", l.Game.ActivePhase())
	}
	if err := runActions(t, l, c, "run 1\n"); err != nil {
		t.Fatalf("RunActions: %v", err)
	}
	if l.Game.Turn() != 1 || l.Game.ActivePhase() != engine.Cleanup {
		t.Errorf("turn %d phase %v after run 1, want turn 1 Cleanup", l.Game.Turn(), l.Game.ActivePhase())
	}
}

func TestRunActionsStepAndRunBadArgumentsError(t *testing.T) {
	t.Parallel()

	for _, line := range []string{"step x", "run", "run x", "run 1 2"} {
		t.Run(line, func(t *testing.T) {
			t.Parallel()

			l := load(t, testDB(t), "humanlife=20\nailife=20\n")
			if err := runActions(t, l, engine.NewScriptedController(), "startturn human\n"+line+"\n"); err == nil {
				t.Errorf("%q did not error", line)
			}
		})
	}
}

// run and step surface the engine's own errors: Run before StartTurn has
// no turn to drive.
func TestRunActionsRunBeforeStartTurnErrors(t *testing.T) {
	t.Parallel()

	l := load(t, testDB(t), "humanlife=20\nailife=20\n")
	if err := runActions(t, l, engine.NewScriptedController(), "run 1\n"); err == nil {
		t.Error("run before startturn did not error")
	}
}
