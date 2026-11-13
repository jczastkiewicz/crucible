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

// Each recorded game replays in Go to the first gap it shows, or to its end. The wants are
// the gaps open today (game-state.md "Not ported yet"); porting one moves its
// game's first divergence later, and the want here moves with it.
func TestReplayFindsTheFirstGapOfEachRecordedGame(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		path string
		// events and err are substrings of the divergence and of the Go
		// game's error, "" for none; a game with neither replays the same.
		events, err string
	}{
		// These games replay to their end with no gap.
		{fixture, "", ""},
		{counterFixture, "", ""},
		{rakdosFixture, "", ""},
		{"../../testdata/replay/domain-zoo-vs-ruby-storm-seed5.jsonl", "", ""},
		{"../../testdata/replay/eldrazi-vs-boros-energy-seed7.jsonl", "", ""},
		{"../../testdata/replay/affinity-vs-izzet-prowess-seed4.jsonl", "", ""},
		{"../../testdata/replay/eldrazi-ramp-vs-broodscale-combo-seed6.jsonl", "", ""},
		{"../../testdata/replay/goryos-vengeance-vs-boros-energy-seed12.jsonl", "", ""},
		{"../../testdata/replay/domain-zoo-vs-eldrazi-seed18.jsonl", "", ""},
		{"../../testdata/replay/affinity-vs-goryos-vengeance-seed17.jsonl", "", ""},
		{"../../testdata/replay/izzet-prowess-vs-eldrazi-ramp-seed15.jsonl", "", ""},
		{"../../testdata/replay/esper-blink-vs-rakdos-midrange-seed13.jsonl", "", ""},
		{"../../testdata/replay/amulet-titan-vs-esper-blink-seed11.jsonl", "", ""},
		{"../../testdata/replay/ruby-storm-vs-azorius-control-seed14.jsonl", "", ""},
	} {
		res, err := replay.Run(corpusDB(t), readLog(t, tc.path), 100)
		if err != nil {
			t.Fatalf("%s: %v", tc.path, err)
		}
		if tc.events == "" && tc.err == "" {
			if !res.Same() || len(res.Go) == 0 {
				t.Errorf("%s: not the same game: question %v, divergence %v, error %v", tc.path, res.Question, res.Events, res.Err)
			}
			continue
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
