package fixture_test

import (
	"strings"
	"testing"

	"github.com/jczastkiewicz/crucible/internal/carddb/compile"
	"github.com/jczastkiewicz/crucible/internal/engine"
)

// menaceCombat is a human Menace creature attacking an AI creature that
// could block it alone: the one-blocker declaration is illegal (CR
// 702.111b), a rejection the engine leaves on the Game (ADR-0024).
const menaceCombat = "humanlife=20\nailife=20\nhumanbattlefield=Sneak|Id:1\naibattlefield=Wall|Id:2\n"

func menaceDB(t *testing.T) *compile.DB {
	t.Helper()
	return castTestDB(t,
		permanentCard{name: "Sneak", typeLine: "Creature", cost: "B", keywords: []string{"Menace"}},
		permanentCard{name: "Wall", typeLine: "Creature", cost: "W"},
	)
}

// expecterror consumes the error an illegal declaration leaves on the Game,
// and the declaration changes nothing: a legal one can follow.
func TestExpectErrorConsumesARejectedDeclaration(t *testing.T) {
	t.Parallel()

	l := load(t, menaceDB(t), menaceCombat)
	c := engine.NewScriptedController()
	log := "startturn human\nadvance 5\nqueue attackers 1\ndeclareattackers\n" +
		"queue blocks 2=1\nexpecterror 509.1b declareblockers\n" +
		"queue blocks none\ndeclareblockers\n"
	if err := runActions(t, l, c, log); err != nil {
		t.Fatalf("RunActions: %v", err)
	}
	if got := l.Game.Blocks(); len(got) != 0 {
		t.Errorf("Blocks() = %v, want none", got)
	}
	if err := l.Game.TakePendingError(); err != nil {
		t.Errorf("pending error left behind: %v", err)
	}
}

// Without expecterror the same declaration fails the run, naming the line.
func TestRejectedDeclarationFailsTheRun(t *testing.T) {
	t.Parallel()

	l := load(t, menaceDB(t), menaceCombat)
	c := engine.NewScriptedController()
	log := "startturn human\nadvance 5\nqueue attackers 1\ndeclareattackers\nqueue blocks 2=1\ndeclareblockers\n"
	err := runActions(t, l, c, log)
	if err == nil || !strings.Contains(err.Error(), "line 6") || !strings.Contains(err.Error(), "cannot be blocked with 1 creatures") {
		t.Fatalf("RunActions = %v, want the line 6 declaration error", err)
	}
}

// An action expecterror expected to fail that succeeds is itself an error,
// as is one failing for another reason, or a line missing the action.
func TestExpectErrorRejectsTheWrongOutcome(t *testing.T) {
	t.Parallel()

	for name, tc := range map[string]struct{ log, want string }{
		"succeeds":   {"startturn human\nadvance 5\nqueue attackers none\nexpecterror 508 declareattackers\n", "succeeded"},
		"wrong text": {"startturn human\nadvance 5\nqueue attackers 1\ndeclareattackers\nqueue blocks 2=1\nexpecterror 508.1d declareblockers\n", "want an error containing"},
		"own error":  {"expecterror nosuchcard playland human 99\n", "want an error containing"},
		"no action":  {"expecterror 509.1b\n", "want a text and an action"},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			l := load(t, menaceDB(t), menaceCombat)
			err := runActions(t, l, engine.NewScriptedController(), tc.log)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("RunActions = %v, want an error containing %q", err, tc.want)
			}
		})
	}
}

// An inner action's own error satisfies expecterror when the text matches.
func TestExpectErrorAcceptsTheActionsOwnError(t *testing.T) {
	t.Parallel()

	l := load(t, menaceDB(t), menaceCombat)
	if err := runActions(t, l, engine.NewScriptedController(), "expecterror playland playland human 99\n"); err != nil {
		t.Fatalf("RunActions: %v", err)
	}
}
