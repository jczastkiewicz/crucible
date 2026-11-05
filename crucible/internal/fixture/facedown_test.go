package fixture_test

import (
	"strings"
	"testing"

	"github.com/jczastkiewicz/crucible/internal/carddb"
	"github.com/jczastkiewicz/crucible/internal/carddb/compile"
	"github.com/jczastkiewicz/crucible/internal/cardtype"
	"github.com/jczastkiewicz/crucible/internal/engine"
	"github.com/jczastkiewicz/crucible/internal/fixture"
)

// bearDB is a database holding one vanilla creature, Test Bear.
func bearDB(t *testing.T) *compile.DB {
	t.Helper()
	raw := &carddb.Card{Filename: "test_bear"}
	raw.Faces[0] = carddb.Face{Present: true, Name: "Test Bear", Type: cardtype.ParseToken("Creature"), Power: "2", Toughness: "2"}
	c, err := compile.Compile(raw)
	if err != nil {
		t.Fatalf("compile bear: %v", err)
	}
	return compile.NewDB(map[string]*compile.Card{"Test Bear": c})
}

// FaceDown (GameState.java:1338-1339) loads a permanent and an exiled card
// face down, and both dump back under the card's own name with |FaceDown
// (GameState.java:342-343, :440-441).
func TestFaceDownRoundTrips(t *testing.T) {
	t.Parallel()

	l := load(t, bearDB(t), "humanlife=20\nailife=20\nhumanbattlefield=Test Bear|Id:1|FaceDown\nhumanexile=Test Bear|Id:2|FaceDown\n")
	for _, n := range []int{1, 2} {
		if !l.Game.Card(l.CardByFixtureID[n]).IsFaceDown() {
			t.Errorf("card %d is face up, want face down", n)
		}
	}
	var buf strings.Builder
	if err := fixture.Write(&buf, fixture.Dump(l)); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if got := strings.Count(buf.String(), "Test Bear|Id:"); got != 2 {
		t.Errorf("dump %q names Test Bear %d times, want 2", buf.String(), got)
	}
	if got := strings.Count(buf.String(), "|FaceDown"); got != 2 {
		t.Errorf("dump %q has %d FaceDown entries, want 2", buf.String(), got)
	}
}

// FaceDown on a card in a hidden zone has nothing to turn: an error.
func TestFaceDownInHandIsAnError(t *testing.T) {
	t.Parallel()

	st, err := fixture.Parse(strings.NewReader("humanlife=20\nailife=20\nhumanhand=Test Bear|FaceDown\n"))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if _, err := fixture.Load(st, bearDB(t), nil); err == nil {
		t.Error("FaceDown in the hand loaded")
	}
}

// queue assistant names the player an Assist spell's caster asks, queue
// assistamount what they pay; a non-number amount is an error.
func TestAssistQueueVerbs(t *testing.T) {
	t.Parallel()

	l := load(t, bearDB(t), "humanlife=20\nailife=20\n")
	c := engine.NewScriptedController()
	if err := runActions(t, l, c, "queue assistant ai\nqueue assistamount 2\n"); err != nil {
		t.Fatalf("runActions: %v", err)
	}
	human, ai := l.Game.Players()[0], l.Game.Players()[1]
	if got := c.ChooseAssistant(l.Game, human, engine.NoCard, []engine.PlayerID{ai}, 2); got != ai {
		t.Errorf("assistant %v, want ai", got)
	}
	if got := c.AssistAmount(l.Game, ai, engine.NoCard, 2); got != 2 {
		t.Errorf("assist amount %d, want 2", got)
	}
	for _, line := range []string{"queue assistamount two", "queue assistant nobody"} {
		if err := runActions(t, l, engine.NewScriptedController(), line+"\n"); err == nil {
			t.Errorf("%q did not error", line)
		}
	}
}

// queue coincall answers the next coin call, heads or tails.
func TestCoinCallQueueVerb(t *testing.T) {
	t.Parallel()

	l := load(t, bearDB(t), "humanlife=20\nailife=20\n")
	c := engine.NewScriptedController()
	if err := runActions(t, l, c, "queue coincall tails\nqueue coincall heads\n"); err != nil {
		t.Fatalf("runActions: %v", err)
	}
	human := l.Game.Players()[0]
	if c.CallCoinFlip(l.Game, human, engine.NoCard) || !c.CallCoinFlip(l.Game, human, engine.NoCard) {
		t.Error("coin calls, want tails then heads")
	}
	if err := runActions(t, l, engine.NewScriptedController(), "queue coincall edge\n"); err == nil {
		t.Error("queue coincall edge did not error")
	}
}
