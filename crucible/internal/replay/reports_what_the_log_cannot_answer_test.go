package replay_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/jczastkiewicz/crucible/internal/replay"
)

// mutated is the fixture log with edit applied to each record after the
// header; a record edit returns nil is dropped and the rest renumbered.
func mutated(t *testing.T, path string, edit func(rec map[string]any) map[string]any) *replay.Log {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(raw)), "\n")
	out := []string{lines[0]}
	for _, l := range lines[1:] {
		var rec map[string]any
		if err := json.Unmarshal([]byte(l), &rec); err != nil {
			t.Fatal(err)
		}
		if rec = edit(rec); rec == nil {
			continue
		}
		rec["seq"] = len(out) - 1
		b, err := json.Marshal(rec)
		if err != nil {
			t.Fatal(err)
		}
		out = append(out, string(b))
	}
	var header map[string]any
	if err := json.Unmarshal([]byte(out[0]), &header); err != nil {
		t.Fatal(err)
	}
	header["records"] = len(out) - 1
	b, err := json.Marshal(header)
	if err != nil {
		t.Fatal(err)
	}
	out[0] = string(b)
	log, err := replay.Read(strings.NewReader(strings.Join(out, "\n") + "\n"))
	if err != nil {
		t.Fatal(err)
	}
	return log
}

// played is whether a call record is one of the calls that play the game
// forward: the starting player, mulligans, priority and payment.
func played(r map[string]any) bool {
	switch r["method"] {
	case "chooseStartingPlayer", "mulliganKeepHand", "chooseSpellAbilityToPlay", "playChosenSpellAbility", "payManaCost":
		return true
	}
	return false
}

// A log that does not answer a question Go asks is reported with the first
// such question, and the Go game still plays on with the default answers.
func TestReplayReportsTheFirstQuestionTheLogCannotAnswer(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name string
		edit func(map[string]any) map[string]any
		// methods are the first unanswered question's, nil for any.
		methods []string
	}{
		{"calls dropped", func(r map[string]any) map[string]any {
			if r["kind"] == "call" || r["kind"] == "play" || r["kind"] == "payment" {
				return nil
			}
			return r
		}, []string{
			// A seat with a companion is asked for it first (Kaheera).
			"chooseStartingPlayer", "chooseEntitiesForEffect",
		}},
		{"choices dropped", func(r map[string]any) map[string]any {
			if r["kind"] == "call" && !played(r) {
				return nil
			}
			return r
		}, nil},
		{"choices garbled", func(r map[string]any) map[string]any {
			if r["kind"] == "call" && !played(r) {
				r["answer"] = "garbled"
			}
			return r
		}, nil},
		{"choices name no card", func(r map[string]any) map[string]any {
			if r["kind"] == "call" && !played(r) {
				r["answer"] = []any{map[string]any{"card": 99999, "name": "Nowhere"}}
			}
			return r
		}, nil},
		{"plays name no card", func(r map[string]any) map[string]any {
			if r["kind"] == "call" && r["method"] == "chooseSpellAbilityToPlay" && r["answer"] != nil {
				r["answer"] = []any{map[string]any{"host": map[string]any{"card": 99999, "name": "Nowhere"}, "section": "land"}}
			}
			return r
		}, []string{"chooseSpellAbilityToPlay"}},
	} {
		paths, err := filepath.Glob("../../testdata/replay/*.jsonl")
		if err != nil {
			t.Fatal(err)
		}
		reported := 0
		for _, path := range paths {
			res, err := replay.Run(corpusDB(t), mutated(t, path, tc.edit), 30)
			if err != nil {
				t.Fatalf("%s, %s: %v", tc.name, path, err)
			}
			if res.Question == nil {
				// The Go game stopped on its own error before Go asked a
				// question the edit took away.
				if res.Err == nil {
					t.Errorf("%s, %s: no unanswered question and no error", tc.name, path)
				}
				continue
			}
			reported++
			if tc.methods != nil && !slices.Contains(tc.methods, res.Question.Method) {
				t.Errorf("%s, %s: first unanswered %v, want one of %q", tc.name, path, res.Question, tc.methods)
			}
			if !strings.Contains(res.Question.String(), res.Question.Method) || res.Same() {
				t.Errorf("%s, %s: report %q", tc.name, path, res.Question)
			}
		}
		if reported == 0 {
			t.Errorf("%s: no game reported an unanswered question", tc.name)
		}
	}
}

// A log the Go game cannot be set up from is an error, not a result.
func TestReplayRefusesALogItCannotSetUp(t *testing.T) {
	t.Parallel()

	log := readLog(t, fixture)
	bad := *log
	bad.Header.StartingLibraries = bad.Header.StartingLibraries[:1]
	if _, err := replay.Run(corpusDB(t), &bad, 1); err == nil || !strings.Contains(err.Error(), "starting libraries") {
		t.Errorf("one starting library for two seats: err %v", err)
	}

	bad = *log
	bad.Header.Seats = append([]replay.Seat(nil), log.Header.Seats...)
	bad.Header.Seats[0].Cards = append([]replay.Ref{replay.CardRef(1, "No Such Card")}, log.Header.Seats[0].Cards[1:]...)
	if _, err := replay.Run(corpusDB(t), &bad, 1); err == nil || !strings.Contains(err.Error(), "No Such Card") {
		t.Errorf("unknown card: err %v", err)
	}

	broken := mutated(t, fixture, func(r map[string]any) map[string]any {
		if r["kind"] == "call" {
			r["decider"] = "not a seat"
		}
		return r
	})
	if _, err := replay.Run(corpusDB(t), broken, 1); err == nil {
		t.Error("a call record that does not decode: no error")
	}
}
