package replay_test

import (
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"

	"github.com/jczastkiewicz/crucible/internal/carddb/compile"
	"github.com/jczastkiewicz/crucible/internal/replay"
)

var (
	dbOnce sync.Once
	dbVal  *compile.DB
	dbErr  error
)

// corpusDB loads the real corpus once: a recorded game names corpus cards.
func corpusDB(t *testing.T) *compile.DB {
	t.Helper()
	dbOnce.Do(func() {
		_, file, _, _ := runtime.Caller(0)
		root := filepath.Join(filepath.Dir(file), "..", "..", "..")
		types := filepath.Join(root, "forge-gui", "res", "lists", "TypeLists.txt")
		dbVal, dbErr = compile.LoadDB(filepath.Join(root, "forge-gui", "res", "cardsfolder"), types)
		if dbErr != nil {
			return
		}
		var tokens map[string]*compile.Card
		if tokens, dbErr = compile.LoadTokenScripts(filepath.Join(root, "forge-gui", "res", "tokenscripts"), types); dbErr == nil {
			dbVal = dbVal.WithTokens(tokens)
		}
	})
	if dbErr != nil {
		t.Fatalf("load corpus: %v", dbErr)
	}
	return dbVal
}

// Each recorded game replays in Go to the first gap it shows. The wants are
// the gaps open today (game-state.md "Not ported yet"); porting one moves its
// game's first divergence later, and the want here moves with it.
func TestReplayFindsTheFirstGapOfEachRecordedGame(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		path string
		// events and err are substrings of the divergence and of the Go
		// game's error, "" for none.
		events, err string
	}{
		// Companion setup: Java moves Kaheera to the command zone before the
		// first turn; Go does not, and Go cannot cast Solitude for its evoke
		// cost.
		{fixture, "move card 143 (Kaheera, the Orphanguard) Sideboard -> Command", "could not cast card 122"},
		{counterFixture, "move card 69 (Kaheera, the Orphanguard) Sideboard -> Command", "could not cast card 46"},
		// Go puts Seasoned Pyromancer's enters trigger on the stack while the
		// spell resolves, Java once it has resolved (CR 603.3, ADR-0019's
		// inline triggers); every question is answered and the game plays on.
		{rakdosFixture,
			"java only:\n    resolve card 53 (Seasoned Pyromancer)\n  go only:\n    push card 53 (Seasoned Pyromancer) by seat 0", ""},
		// The same inline triggers: Commercial District's, Haliya's. Go then
		// offers no Ral as a target (domain zoo) and no Broodscale to choose
		// (Eldrazi).
		{"../../testdata/replay/domain-zoo-vs-ruby-storm-seed5.jsonl",
			"go only:\n    push card 85 (Commercial District) by seat 1", ""},
		{"../../testdata/replay/eldrazi-vs-boros-energy-seed7.jsonl",
			"go only:\n    push card 103 (Haliya, Guided by Light) by seat 1", "Dig: controller chose 35"},
		// Java's Equip chose its creature as it resolved, not as a target.
		{"../../testdata/replay/affinity-vs-izzet-prowess-seed4.jsonl",
			"go only:\n    push card 55 (Weapons Manufacturing) by seat 0", "could not cast card 18"},
		// Java paid Basking Broodscale with Eldrazi Temple's colorless-Eldrazi
		// mana as the log names it; Go refuses that payment.
		{"../../testdata/replay/eldrazi-ramp-vs-broodscale-combo-seed6.jsonl",
			"java only:\n    move card 80 (Basking Broodscale) Hand -> Stack", "could not cast card 80"},
	} {
		res, err := replay.Run(corpusDB(t), readLog(t, tc.path), 60)
		if err != nil {
			t.Fatalf("%s: %v", tc.path, err)
		}
		if res.Events == nil || !strings.Contains(res.Events.String(), tc.events) {
			t.Errorf("%s: divergence %v, want one with %q", tc.path, res.Events, tc.events)
		}
		switch {
		case tc.err == "" && res.Err != nil:
			t.Errorf("%s: Go game error %v", tc.path, res.Err)
		case tc.err != "" && (res.Err == nil || !strings.Contains(res.Err.Error(), tc.err)):
			t.Errorf("%s: Go game error %v, want one with %q", tc.path, res.Err, tc.err)
		}
		if res.Same() || len(res.Go) == 0 || res.Turns == 0 {
			t.Errorf("%s: same %v, %d Go events, %d turns", tc.path, res.Same(), len(res.Go), res.Turns)
		}
	}
}
