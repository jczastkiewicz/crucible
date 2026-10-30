package fixture_test

import (
	"strings"
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
)

// foretell and plot name a player and exactly one card; a missing or unknown
// card id is a fixture error naming the verb, not an engine call on a bad
// handle. Queued as an action, each takes exactly one card id.
func TestSpecialActionsRejectABadCardArgument(t *testing.T) {
	t.Parallel()

	for _, verb := range []string{"foretell", "plot"} {
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

// A queued foretell or plot with one card id parses as an action the
// controller takes when next given priority.
func TestQueuedSpecialActionsParse(t *testing.T) {
	t.Parallel()

	for _, verb := range []string{"foretell", "plot"} {
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
