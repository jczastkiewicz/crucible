package fixture_test

import (
	"strings"
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
	"github.com/jczastkiewicz/crucible/internal/fixture"
)

func runActions(t *testing.T, l *fixture.Loaded, c *engine.ScriptedController, log string) error {
	t.Helper()
	return fixture.RunActions(strings.NewReader(log), l, c)
}

func TestRunActionsStartTurnAndAdvance(t *testing.T) {
	t.Parallel()

	db := testDB(t)
	l := load(t, db, "humanlife=20\nailife=20\n")
	c := engine.NewScriptedController()

	if err := runActions(t, l, c, "startturn human\nadvance\nadvance 2\n"); err != nil {
		t.Fatalf("RunActions: %v", err)
	}

	if l.Game.Turn() != 1 {
		t.Errorf("turn %d, want 1", l.Game.Turn())
	}
	// Untap (startturn) -> Upkeep (advance) -> Draw -> Main1 (advance 2).
	if l.Game.ActivePhase() != engine.Main1 {
		t.Errorf("phase %v, want Main1", l.Game.ActivePhase())
	}
}

// Comments and blank lines are noise, the same convention setup.state uses.
func TestRunActionsSkipsCommentsAndBlankLines(t *testing.T) {
	t.Parallel()

	db := testDB(t)
	l := load(t, db, "humanlife=20\nailife=20\n")
	c := engine.NewScriptedController()

	err := runActions(t, l, c, "# a comment\n\nstartturn human\n\n# another\n")
	if err != nil {
		t.Fatalf("RunActions: %v", err)
	}
	if l.Game.Turn() != 1 {
		t.Errorf("turn %d, want 1", l.Game.Turn())
	}
}

// A full mulligan exchange, scripted end to end: queue the decisions and the
// tuck before the action that consumes them, the same order
// ScriptedController expects. Id: on every hand card is what lets
// `queue tuck` name one without knowing which CardID a shuffle puts where --
// the hand is a closed pool of seven known ids the whole time.
func TestRunActionsMulliganWithQueuedKeepHand(t *testing.T) {
	t.Parallel()

	db := testDB(t, "Mountain")
	l := load(t, db, "humanlife=20\nailife=20\n"+
		"humanhand=Mountain|Id:1;Mountain|Id:2;Mountain|Id:3;Mountain|Id:4;"+
		"Mountain|Id:5;Mountain|Id:6;Mountain|Id:7\n")
	c := engine.NewScriptedController()

	err := runActions(t, l, c, ""+
		"queue keephand false\n"+ // human: mulligan
		"queue keephand true\n"+ // ai: keep (asked in the same round)
		"queue tuck 1\n"+ // the one card human's mulligan will tuck
		"queue keephand true\n"+ // human: keep the redrawn hand
		"mulligan human\n")
	if err != nil {
		t.Fatalf("RunActions: %v", err)
	}

	if got := l.Game.Zone(engine.Hand, l.Game.Players()[0]).Len(); got != 6 {
		t.Errorf("human's hand has %d cards, want 6 (one non-free mulligan)", got)
	}
	if id, ok := l.CardByFixtureID[1]; !ok || l.Game.Card(id).Zone != engine.Library {
		t.Error("the tucked card is not in the library")
	}
}

// queue tuck resolves setup.state's Id: numbers to the CardIDs Load actually
// assigned, so a scenario can name a specific card without knowing its
// handle ahead of time.
func TestRunActionsQueueTuckResolvesFixtureID(t *testing.T) {
	t.Parallel()

	db := testDB(t, "Mountain")
	l := load(t, db, "humanlife=20\nailife=20\nhumanbattlefield=Mountain|Id:7\n")
	want, ok := l.CardByFixtureID[7]
	if !ok {
		t.Fatal("setup: Id:7 did not resolve to a CardID")
	}
	c := engine.NewScriptedController()

	if err := runActions(t, l, c, "queue tuck 7\n"); err != nil {
		t.Fatalf("RunActions: %v", err)
	}

	got := c.TuckCardsViaMulligan(l.Game, l.Game.Players()[0], nil, 1)
	if len(got) != 1 || got[0] != want {
		t.Errorf("tucked %v, want [%v]", got, want)
	}
}

// queue tuck accepts more than one id, comma-separated.
func TestRunActionsQueueTuckMultipleIDs(t *testing.T) {
	t.Parallel()

	db := testDB(t, "Mountain", "Forest")
	l := load(t, db, "humanlife=20\nailife=20\nhumanbattlefield=Mountain|Id:1;Forest|Id:2\n")
	c := engine.NewScriptedController()

	if err := runActions(t, l, c, "queue tuck 1,2\n"); err != nil {
		t.Fatalf("RunActions: %v", err)
	}

	got := c.TuckCardsViaMulligan(l.Game, l.Game.Players()[0], nil, 2)
	want := []engine.CardID{l.CardByFixtureID[1], l.CardByFixtureID[2]}
	if len(got) != 2 || got[0] != want[0] || got[1] != want[1] {
		t.Errorf("tucked %v, want %v", got, want)
	}
}

// queue legendarykeep resolves setup.state's Id: number the same way tuck
// does, for the one card ChooseLegendaryToKeep should return.
func TestRunActionsQueueLegendaryKeepResolvesFixtureID(t *testing.T) {
	t.Parallel()

	db := testDB(t, "Mountain")
	l := load(t, db, "humanlife=20\nailife=20\nhumanbattlefield=Mountain|Id:7\n")
	want, ok := l.CardByFixtureID[7]
	if !ok {
		t.Fatal("setup: Id:7 did not resolve to a CardID")
	}
	c := engine.NewScriptedController()

	if err := runActions(t, l, c, "queue legendarykeep 7\n"); err != nil {
		t.Fatalf("RunActions: %v", err)
	}

	got := c.ChooseLegendaryToKeep(l.Game, l.Game.Players()[0], nil)
	if got != want {
		t.Errorf("legendary to keep = %v, want %v", got, want)
	}
}

// queue legendarykeep takes exactly one id -- unlike tuck, there is only
// ever one permanent to keep.
func TestRunActionsQueueLegendaryKeepWantsExactlyOneID(t *testing.T) {
	t.Parallel()

	db := testDB(t, "Mountain", "Forest")
	l := load(t, db, "humanlife=20\nailife=20\nhumanbattlefield=Mountain|Id:1;Forest|Id:2\n")
	c := engine.NewScriptedController()

	if err := runActions(t, l, c, "queue legendarykeep 1,2\n"); err == nil {
		t.Error("two ids did not error")
	}
}

func TestRunActionsQueueLegendaryKeepBadIDErrors(t *testing.T) {
	t.Parallel()

	db := testDB(t)
	l := load(t, db, "humanlife=20\n")
	c := engine.NewScriptedController()

	if err := runActions(t, l, c, "queue legendarykeep abc\n"); err == nil {
		t.Error("a non-numeric id did not error")
	}
}

// declareattackers runs with nothing on the battlefield without error --
// there is nothing eligible, so Game.DeclareCombatAttackers never touches
// the controller's queue at all.
func TestRunActionsDeclareAttackersWithNothingOnTheBattlefield(t *testing.T) {
	t.Parallel()

	db := testDB(t)
	l := load(t, db, "humanlife=20\nailife=20\n")
	c := engine.NewScriptedController()

	if err := runActions(t, l, c, "startturn human\ndeclareattackers\n"); err != nil {
		t.Fatalf("RunActions: %v", err)
	}
}

// queue attackers resolves setup.state's Id: numbers the same way tuck and
// legendarykeep do.
func TestRunActionsQueueAttackersResolvesFixtureIDs(t *testing.T) {
	t.Parallel()

	db := testDB(t, "Mountain", "Forest")
	l := load(t, db, "humanlife=20\nailife=20\nhumanbattlefield=Mountain|Id:1;Forest|Id:2\n")
	c := engine.NewScriptedController()

	if err := runActions(t, l, c, "queue attackers 1,2\n"); err != nil {
		t.Fatalf("RunActions: %v", err)
	}

	got := c.DeclareCombatAttackers(l.Game, l.Game.Players()[0], nil)
	want := []engine.CardID{l.CardByFixtureID[1], l.CardByFixtureID[2]}
	if len(got) != 2 || got[0] != want[0] || got[1] != want[1] {
		t.Errorf("attackers %v, want %v", got, want)
	}
}

// queue attackers none is how a scenario queues "decline to attack" --
// there being no ids is not the same as the line being absent, since every
// other queue kind also requires a value.
func TestRunActionsQueueAttackersNoneDeclines(t *testing.T) {
	t.Parallel()

	db := testDB(t)
	l := load(t, db, "humanlife=20\nailife=20\n")
	c := engine.NewScriptedController()

	if err := runActions(t, l, c, "queue attackers none\n"); err != nil {
		t.Fatalf("RunActions: %v", err)
	}

	got := c.DeclareCombatAttackers(l.Game, l.Game.Players()[0], nil)
	if got != nil {
		t.Errorf("attackers = %v, want nil", got)
	}
}

func TestRunActionsQueueAttackersBadIDErrors(t *testing.T) {
	t.Parallel()

	db := testDB(t)
	l := load(t, db, "humanlife=20\n")
	c := engine.NewScriptedController()

	if err := runActions(t, l, c, "queue attackers abc\n"); err == nil {
		t.Error("a non-numeric id did not error")
	}
}

func TestRunActionsUnknownVerbErrors(t *testing.T) {
	t.Parallel()

	db := testDB(t)
	l := load(t, db, "humanlife=20\n")
	c := engine.NewScriptedController()

	if err := runActions(t, l, c, "castspell human\n"); err == nil {
		t.Error("an unknown verb ran without error")
	}
}

func TestRunActionsUnknownQueueKindErrors(t *testing.T) {
	t.Parallel()

	db := testDB(t)
	l := load(t, db, "humanlife=20\n")
	c := engine.NewScriptedController()

	if err := runActions(t, l, c, "queue targetplayer human\n"); err == nil {
		t.Error("an unknown queue kind ran without error")
	}
}

func TestRunActionsUnknownPlayerErrors(t *testing.T) {
	t.Parallel()

	db := testDB(t)
	l := load(t, db, "humanlife=20\nailife=20\n")
	c := engine.NewScriptedController()

	if err := runActions(t, l, c, "startturn nobody\n"); err == nil {
		t.Error("an unseated player name ran without error")
	}
}

func TestRunActionsMulliganUnknownPlayerErrors(t *testing.T) {
	t.Parallel()

	db := testDB(t)
	l := load(t, db, "humanlife=20\nailife=20\n")
	c := engine.NewScriptedController()

	if err := runActions(t, l, c, "mulligan nobody\n"); err == nil {
		t.Error("mulligan naming an unseated player ran without error")
	}
}

func TestRunActionsQueueStartingPlayerUnknownPlayerErrors(t *testing.T) {
	t.Parallel()

	db := testDB(t)
	l := load(t, db, "humanlife=20\nailife=20\n")
	c := engine.NewScriptedController()

	if err := runActions(t, l, c, "queue startingplayer nobody\n"); err == nil {
		t.Error("queue startingplayer naming an unseated player ran without error")
	}
}

func TestRunActionsMalformedAdvanceCountErrors(t *testing.T) {
	t.Parallel()

	db := testDB(t)
	l := load(t, db, "humanlife=20\n")
	c := engine.NewScriptedController()

	if err := runActions(t, l, c, "advance many\n"); err == nil {
		t.Error("a non-numeric advance count ran without error")
	}
}

func TestRunActionsMalformedKeepHandErrors(t *testing.T) {
	t.Parallel()

	db := testDB(t)
	l := load(t, db, "humanlife=20\n")
	c := engine.NewScriptedController()

	if err := runActions(t, l, c, "queue keephand maybe\n"); err == nil {
		t.Error("a non-boolean keephand value ran without error")
	}
}

func TestRunActionsMalformedStartingHandErrors(t *testing.T) {
	t.Parallel()

	db := testDB(t)
	l := load(t, db, "humanlife=20\n")
	c := engine.NewScriptedController()

	if err := runActions(t, l, c, "queue startinghand first\n"); err == nil {
		t.Error("a non-numeric startinghand index ran without error")
	}
}

func TestRunActionsTuckMalformedIDErrors(t *testing.T) {
	t.Parallel()

	db := testDB(t)
	l := load(t, db, "humanlife=20\n")
	c := engine.NewScriptedController()

	if err := runActions(t, l, c, "queue tuck abc\n"); err == nil {
		t.Error("a non-numeric tuck id ran without error")
	}
}

func TestRunActionsTuckUnknownIDErrors(t *testing.T) {
	t.Parallel()

	db := testDB(t)
	l := load(t, db, "humanlife=20\n")
	c := engine.NewScriptedController()

	if err := runActions(t, l, c, "queue tuck 99\n"); err == nil {
		t.Error("a tuck id naming no card ran without error")
	}
}

func TestRunActionsQueueTooFewArgsErrors(t *testing.T) {
	t.Parallel()

	db := testDB(t)
	l := load(t, db, "humanlife=20\n")
	c := engine.NewScriptedController()

	if err := runActions(t, l, c, "queue keephand\n"); err == nil {
		t.Error("a queue line with no value ran without error")
	}
}

func TestRunActionsQueueStartingPlayerAndHand(t *testing.T) {
	t.Parallel()

	db := testDB(t)
	l := load(t, db, "humanlife=20\nailife=20\n")
	c := engine.NewScriptedController()

	err := runActions(t, l, c, "queue startingplayer ai\nqueue startinghand 1\n")
	if err != nil {
		t.Fatalf("RunActions: %v", err)
	}

	if got := c.ChooseStartingPlayer(l.Game, l.Game.Players()[0], true); got != l.Game.Players()[1] {
		t.Errorf("starting player %v, want ai", got)
	}
	if got := c.ChooseStartingHand(l.Game, l.Game.Players()[0], nil); got != 1 {
		t.Errorf("starting hand index %d, want 1", got)
	}
}

// A line naming too few fields for its verb is a fixture-authoring error --
// the same as everywhere else in this package, that has to fail loud rather
// than panic on a short slice or silently do nothing.
func TestRunActionsStartTurnWithNoPlayerErrors(t *testing.T) {
	t.Parallel()

	db := testDB(t)
	l := load(t, db, "humanlife=20\n")
	c := engine.NewScriptedController()

	if err := runActions(t, l, c, "startturn\n"); err == nil {
		t.Error("startturn with no player ran without error")
	}
}
