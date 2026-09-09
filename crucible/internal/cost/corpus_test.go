package cost_test

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"

	"github.com/jczastkiewicz/crucible/internal/carddb"
	"github.com/jczastkiewicz/crucible/internal/carddb/vocab"
	"github.com/jczastkiewicz/crucible/internal/cardtype"
	"github.com/jczastkiewicz/crucible/internal/cost"
)

var update = flag.Bool("update", false, "rewrite golden files")

// TestCorpusCosts parses every `Cost$` the corpus writes.
//
// The golden is the inventory of what a payment engine owes: which named parts
// appear, how many fields each body carries, and what falls through to mana.
// The mana list is the interesting half -- Java treats an unrecognised token as
// mana, so a part this table is missing does not fail anywhere, it quietly
// becomes a mana symbol that the mana parser will reject much later.
//
// Regenerate with:
//
//	go test ./internal/cost -run TestCorpusCosts -update
func TestCorpusCosts(t *testing.T) {
	t.Parallel()

	var (
		parsed    int
		parts     = map[string]int{}
		fields    = map[string]int{}
		mana      = map[string]int{}
		roundTrip []string
	)
	forEachCost(t, func(card, value string) {
		parsed++
		c := cost.Parse(value)
		if got := c.String(); got != value {
			roundTrip = append(roundTrip, fmt.Sprintf("%s: %q became %q", card, value, got))
		}
		for _, part := range c.Parts {
			parts[part.Name]++
			fields[fmt.Sprintf("%s\t%d", part.Name, len(part.Fields))]++
		}
		for _, token := range c.Mana {
			mana[token]++
		}
	})

	for i, f := range roundTrip {
		if i == 10 {
			t.Errorf("... and %d more round-trip failures", len(roundTrip)-10)
			break
		}
		t.Errorf("round trip: %s", f)
	}

	if parsed < 10000 {
		t.Fatalf("read %d costs, want at least 10000 -- the walk is broken", parsed)
	}
	t.Logf("parsed %d costs: %d named parts, %d distinct mana tokens", parsed, len(parts), len(mana))

	var lines []string
	for _, name := range sortedKeys(parts) {
		lines = append(lines, fmt.Sprintf("part\t%s\t%d", name, parts[name]))
	}
	for _, name := range sortedKeys(fields) {
		lines = append(lines, fmt.Sprintf("fields\t%s", name))
	}
	for _, name := range sortedKeys(mana) {
		lines = append(lines, fmt.Sprintf("mana\t%s", name))
	}
	compareGolden(t, "testdata/costs.golden", lines)
}

// A mana token is one to three characters, or a hybrid or Phyrexian symbol.
// Anything longer that reaches the mana list is a named part the table does not
// know, which is the failure this catches -- Java would silently treat it as
// mana too.
func TestNothingLongFallsThroughToMana(t *testing.T) {
	t.Parallel()

	var suspicious []string
	forEachCost(t, func(card, value string) {
		for _, token := range cost.Parse(value).Mana {
			if len(token) > 3 && !strings.ContainsAny(token, "/") {
				suspicious = append(suspicious, fmt.Sprintf("%s: %q in %q", card, token, value))
			}
		}
	})
	sort.Strings(suspicious)
	for i, s := range suspicious {
		if i == 10 {
			t.Errorf("... and %d more", len(suspicious)-10)
			break
		}
		t.Errorf("read as mana, but looks like a cost part: %s", s)
	}
}

// forEachCost hands fn every `Cost$` value on every face and variant.
func forEachCost(t *testing.T, fn func(card, value string)) {
	t.Helper()

	reg := testRegistry(t)
	root := filepath.Join(repoRoot(t), "forge-gui", "res", "cardsfolder")
	cards := 0
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || filepath.Ext(path) != ".txt" {
			return nil
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		name := strings.TrimSuffix(filepath.Base(path), ".txt")
		card, err := carddb.ParseScript(reg, name, raw)
		if err != nil {
			return err
		}
		cards++
		for _, i := range card.PresentFaces() {
			eachFace(&card.Faces[i], name, fn)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", root, err)
	}
	if cards < 30000 {
		t.Fatalf("read %d cards, want at least 30000", cards)
	}
}

// eachFace hands fn every `Cost$` that is a cost string.
//
// A `Cost$` on a static ability may instead name an SVar, which
// StaticAbilityCantAttackBlock resolves to a number with calculateAmount before
// it builds a Cost at all (`if (stAb.hasSVar(costString))`). Those are not cost
// strings and parsing them as such would report an SVar name as a mana symbol.
func eachFace(face *carddb.Face, card string, fn func(card, value string)) {
	defined := make(map[string]bool, face.SVars.Len())
	for _, name := range face.SVars.Names() {
		defined[name] = true
	}

	lines := make([]string, 0, 16)
	lines = append(lines, face.Abilities...)
	lines = append(lines, face.Triggers...)
	lines = append(lines, face.Statics...)
	lines = append(lines, face.Replacements...)
	for _, name := range face.SVars.Names() {
		value, _ := face.SVars.Get(name)
		lines = append(lines, value)
	}
	for _, line := range lines {
		for _, p := range vocab.SplitParams(line) {
			if strings.EqualFold(p.Key, "Cost") && !defined[p.Value] {
				fn(card, p.Value)
			}
		}
	}
	for _, variant := range face.Variants {
		eachFace(variant, card, fn)
	}
}

func sortedKeys(m map[string]int) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func testRegistry(t *testing.T) *cardtype.Registry {
	t.Helper()

	path := filepath.Join(repoRoot(t), "forge-gui", "res", "lists", "TypeLists.txt")
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open %s: %v", path, err)
	}
	defer func() { _ = f.Close() }()
	reg, err := cardtype.LoadRegistry(f)
	if err != nil {
		t.Fatalf("load %s: %v", path, err)
	}
	return reg
}

func repoRoot(t *testing.T) string {
	t.Helper()

	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed; cannot locate the corpus")
	}
	// crucible/internal/cost/<file> -> repository root.
	return filepath.Join(filepath.Dir(file), "..", "..", "..")
}

func compareGolden(t *testing.T, path string, lines []string) {
	t.Helper()

	got := strings.Join(lines, "\n") + "\n"
	if *update {
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
		t.Logf("wrote %s (%d lines)", path, len(lines))
		return
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v (regenerate with -update)", path, err)
	}
	want := strings.Split(strings.TrimRight(string(raw), "\n"), "\n")
	shown := 0
	for i := 0; i < len(lines) && i < len(want); i++ {
		if lines[i] != want[i] && shown < 20 {
			t.Errorf("%s line %d:\n got %s\nwant %s", path, i+1, lines[i], want[i])
			shown++
		}
	}
	if len(lines) != len(want) {
		t.Errorf("%s has %d lines, generated %d", path, len(want), len(lines))
	}
}
