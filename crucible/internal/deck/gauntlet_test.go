package deck_test

import (
	"math"
	"os"
	"strings"
	"testing"

	"github.com/jczastkiewicz/crucible/internal/deck"
)

func TestParseGauntletReadsTheManifest(t *testing.T) {
	t.Parallel()
	g, err := deck.ParseGauntlet([]byte(`# a comment
format = "modern"
snapshot = "2026-11-06"
source = "a tracker, \"paper\" results \\ October"

[[deck]]
file = "zoo.dck"
archetype = "Domain Zoo"
weight = 6.0
event = "Regional Championship, 3rd Place, 13-3-1"
date = "Oct 10, 2026"
url = "https://example.org/deck/1"

[[deck]]
file = "tron.dck"
`))
	if err != nil {
		t.Fatal(err)
	}
	want := deck.Gauntlet{
		Format: "modern", Snapshot: "2026-11-06", Source: `a tracker, "paper" results \ October`,
		Decks: []deck.GauntletDeck{
			{File: "zoo.dck", Archetype: "Domain Zoo", Weight: 6, Event: "Regional Championship, 3rd Place, 13-3-1",
				Date: "Oct 10, 2026", URL: "https://example.org/deck/1"},
			{File: "tron.dck"},
		},
	}
	if g.Format != want.Format || g.Snapshot != want.Snapshot || g.Source != want.Source ||
		len(g.Decks) != 2 || g.Decks[0] != want.Decks[0] || g.Decks[1] != want.Decks[1] {
		t.Errorf("got %+v\nwant %+v", g, want)
	}
}

// Whatever the subset does not read is an error, never a skipped line: a
// mistyped key would otherwise drop a weight without a word.
func TestParseGauntletRejectsWhatItDoesNotRead(t *testing.T) {
	t.Parallel()
	head := "format = \"modern\"\nsnapshot = \"2026-11-06\"\nsource = \"s\"\n"
	for name, content := range map[string]string{
		"no format":       "snapshot = \"2026-11-06\"\nsource = \"s\"\n",
		"no source":       "format = \"modern\"\nsnapshot = \"2026-11-06\"\n",
		"unknown key":     head + "notes = \"x\"\n",
		"unknown deck":    head + "[[deck]]\nfile = \"a.dck\"\nwieght = \"1\"\n",
		"unknown table":   head + "[decks]\n",
		"twice":           head + "format = \"pioneer\"\n",
		"twice in a deck": head + "[[deck]]\nfile = \"a.dck\"\nfile = \"b.dck\"\n",
		"no file":         head + "[[deck]]\nweight = 1\n",
		"bad weight":      head + "[[deck]]\nfile = \"a.dck\"\nweight = lots\n",
		"negative":        head + "[[deck]]\nfile = \"a.dck\"\nweight = -1\n",
		"unquoted":        "format = modern\nsnapshot = \"2026-11-06\"\nsource = \"s\"\n",
		"stray quote":     "format = \"mod\"ern\"\nsnapshot = \"2026-11-06\"\nsource = \"s\"\n",
		"bad escape":      "format = \"mod\\nern\"\nsnapshot = \"2026-11-06\"\nsource = \"s\"\n",
		"no equals":       head + "format\n",
	} {
		if _, err := deck.ParseGauntlet([]byte(content)); err == nil {
			t.Errorf("%s: parsed without error", name)
		}
	}
}

// The checked-in Modern snapshot: 14 archetypes whose weights are the
// published shares, 70.7% of the field together.
func TestModernGauntletManifest(t *testing.T) {
	t.Parallel()
	raw, err := os.ReadFile("../../gauntlets/modern/gauntlet.toml")
	if err != nil {
		t.Fatal(err)
	}
	g, err := deck.ParseGauntlet(raw)
	if err != nil {
		t.Fatal(err)
	}
	total := 0.0
	for _, d := range g.Decks {
		total += d.Weight
		if !strings.HasSuffix(d.File, ".dck") || d.Archetype == "" || d.URL == "" {
			t.Errorf("deck %+v lacks a file, archetype or url", d)
		}
	}
	if g.Format != "modern" || len(g.Decks) != 14 || math.Abs(total-70.7) > 1e-9 {
		t.Errorf("format %q, %d decks, weights %.2f: want modern, 14, 70.7", g.Format, len(g.Decks), total)
	}
}
