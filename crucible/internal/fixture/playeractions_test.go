package fixture_test

import (
	"strings"
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
)

// foretell, plot and suspend name a player and exactly one card; a missing or unknown
// card id is a fixture error naming the verb, not an engine call on a bad
// handle. Queued as an action, each takes exactly one card id.
func TestSpecialActionsRejectABadCardArgument(t *testing.T) {
	t.Parallel()

	for _, verb := range []string{"foretell", "plot", "suspend"} {
		for name, line := range map[string]string{
			"no card id":           verb + " human",
			"unknown card":         verb + " human 99",
			"queued, two card ids": "queue action human " + verb + " 1 2",
		} {
			t.Run(verb+" "+name, func(t *testing.T) {
				t.Parallel()

				l := load(t, menaceDB(t), menaceCombat)
				err := runActions(t, l, engine.NewScriptedController(), "startturn human\n"+line+"\n")
				if err == nil || !strings.Contains(err.Error(), verb) {
					t.Fatalf("RunActions(%q) = %v, want an error naming %s", line, err, verb)
				}
			})
		}
	}
}

// A queued foretell, plot or suspend with one card id parses as an action the
// controller takes when next given priority.
func TestQueuedSpecialActionsParse(t *testing.T) {
	t.Parallel()

	for _, verb := range []string{"foretell", "plot", "suspend"} {
		t.Run(verb, func(t *testing.T) {
			t.Parallel()

			l := load(t, menaceDB(t), menaceCombat)
			if err := runActions(t, l, engine.NewScriptedController(), "startturn human\nqueue action human "+verb+" 1\n"); err != nil {
				t.Fatalf("queue action human %s 1: %v", verb, err)
			}
		})
	}
}

// concede takes the named player out of the game; naming nobody is an error.
func TestConcedeRemovesThePlayerOrRejectsTheName(t *testing.T) {
	t.Parallel()

	l := load(t, menaceDB(t), menaceCombat)
	if err := runActions(t, l, engine.NewScriptedController(), "startturn human\nconcede ai\n"); err != nil {
		t.Fatalf("concede ai: %v", err)
	}

	l = load(t, menaceDB(t), menaceCombat)
	if err := runActions(t, l, engine.NewScriptedController(), "concede nobody\n"); err == nil {
		t.Fatal("concede nobody: want an error")
	}
}

// turnfaceup names a player, one card and an optional way index; a missing,
// unknown or extra argument, or a way that is not a number, is a fixture
// error naming the verb. Queued, it parses with or without the way.
func TestTurnFaceUpArguments(t *testing.T) {
	t.Parallel()

	for name, line := range map[string]string{
		"no card id":          "turnfaceup human",
		"unknown card":        "turnfaceup human 99",
		"way not a number":    "turnfaceup human 1 x",
		"too many arguments":  "turnfaceup human 1 0 0",
		"queued, no card id":  "queue action human turnfaceup",
		"queued, way not int": "queue action human turnfaceup 1 x",
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			l := load(t, menaceDB(t), menaceCombat)
			err := runActions(t, l, engine.NewScriptedController(), "startturn human\n"+line+"\n")
			if err == nil || !strings.Contains(err.Error(), "turnfaceup") {
				t.Fatalf("RunActions(%q) = %v, want an error naming turnfaceup", line, err)
			}
		})
	}
	for _, line := range []string{"turnfaceup human 1", "turnfaceup human 1 0", "queue action human turnfaceup 1", "queue action human turnfaceup 1 0"} {
		l := load(t, menaceDB(t), menaceCombat)
		if err := runActions(t, l, engine.NewScriptedController(), "startturn human\n"+line+"\n"); err != nil {
			t.Errorf("%s: %v", line, err)
		}
	}
}
