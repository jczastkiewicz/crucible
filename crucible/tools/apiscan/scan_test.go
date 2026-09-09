package main

import (
	"path/filepath"
	"testing"
)

// Internal test, deliberately. apiscan is a command, so readParams and
// readExclusions are unexported and there is no public API to test through
// (TEST-2). Both are worth pinning: readParams is the whole premise -- a shape
// it misses becomes a card falsely reported as writing a dead param -- and
// readExclusions decides what the gate is allowed to stay quiet about.

func TestReadParams(t *testing.T) {
	t.Parallel()

	got, err := readParams(filepath.Join("testdata", "Reads.java"))
	if err != nil {
		t.Fatalf("readParams: %v", err)
	}

	want := []string{
		"Accessor", "Presence", "Defaulted", // accessors
		"RawContains", "RawGet", // raw map
		"OneHelperKey", "FirstOfTwo", "SecondOfTwo", // helpers
		"BoundToVariable", // key in a variable
		"ValidMatched",    // trigger and replacement matching
	}
	for _, key := range want {
		if !got[key] {
			t.Errorf("readParams missed %q", key)
		}
	}

	// A string literal that is not a param must not be collected, or the known
	// set grows and the gate stops finding anything.
	for _, key := range []string{"NotAParam", "AlsoNotAParam"} {
		if got[key] {
			t.Errorf("readParams collected %q, which no accessor reads", key)
		}
	}

	if len(got) != len(want) {
		t.Errorf("readParams found %d keys, want %d: %v", len(got), len(want), got)
	}
}

func TestReadExclusions(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		file  string
		want  []string
		avoid []string
	}{
		{
			name:  "param key rows only",
			file:  "exclusions.md",
			want:  []string{"excludedkey", "plainexcludedkey"},
			avoid: []string{"wrongkind", "—"},
		},
		{
			name: "no table",
			file: "empty.md",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := readExclusions(filepath.Join("testdata", tt.file))
			if err != nil {
				t.Fatalf("readExclusions: %v", err)
			}
			for _, key := range tt.want {
				if !got[key] {
					t.Errorf("readExclusions missed %q", key)
				}
			}
			// A row of another kind must not silence a param key of the same
			// name, which is why the gate matches on both columns.
			for _, key := range tt.avoid {
				if got[key] {
					t.Errorf("readExclusions took %q, which is not a param-key row", key)
				}
			}
			if len(got) != len(tt.want) {
				t.Errorf("readExclusions found %d rows, want %d: %v", len(got), len(tt.want), got)
			}
		})
	}
}
