package fixture_test

import (
	"strings"
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
)

// foretell names a player and exactly one card; a missing or unknown card id
// is a fixture error naming the verb, not an engine call on a bad handle.
func TestForetellRejectsABadCardArgument(t *testing.T) {
	t.Parallel()

	for name, line := range map[string]string{
		"no card id":   "foretell human",
		"unknown card": "foretell human 99",
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			l := load(t, menaceDB(t), menaceCombat)
			err := runActions(t, l, engine.NewScriptedController(), "startturn human\n"+line+"\n")
			if err == nil || !strings.Contains(err.Error(), "foretell") {
				t.Fatalf("RunActions(%q) = %v, want an error naming foretell", line, err)
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
