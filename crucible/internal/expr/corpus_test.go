package expr_test

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
	"github.com/jczastkiewicz/crucible/internal/expr"
)

var update = flag.Bool("update", false, "rewrite golden files")

// TestCorpusExpressions parses every amount expression the corpus writes: each
// SVar body that is a measurement rather than an ability, and every `Count$`
// inside a param value.
//
// The golden is the inventory M3 still owes an implementation: heads, the
// operators applied to them, and the count heads underneath. It moves when the
// language does, which is the signal, and not when a card is added.
//
// Regenerate with:
//
//	go test ./internal/expr -run TestCorpusExpressions -update
func TestCorpusExpressions(t *testing.T) {
	t.Parallel()

	var (
		parsed     int
		heads      = map[string]int{}
		operators  = map[string]int{}
		countHeads = map[string]int{}
		contexts   = map[string]int{}
		unknownOps []string
		roundTrip  []string
	)
	forEachAmount(t, func(card, text string) {
		amount := expr.Parse(text)
		parsed++
		if got := amount.String(); got != text {
			roundTrip = append(roundTrip, fmt.Sprintf("%s: %q became %q", card, text, got))
		}
		if amount.Kind != expr.Expression {
			return
		}

		heads[amount.Head]++
		if amount.Context != "" {
			contexts[amount.Context]++
		}
		if amount.Op != nil {
			operators[amount.Op.Name]++
			// doXMath leaves the number alone when it recognises nothing, so
			// an unmatched operator is silently a no-op rather than an error.
			if _, ok := expr.Operator(amount.Op.Name); !ok {
				unknownOps = append(unknownOps, fmt.Sprintf("%s: %q applies %q", card, text, amount.Op.Name))
			}
		}
		if amount.Head == "Count" {
			countHeads[expr.ParseCount(amount.Body).Head]++
		}
	})

	for i, f := range roundTrip {
		if i == 10 {
			t.Errorf("... and %d more round-trip failures", len(roundTrip)-10)
			break
		}
		t.Errorf("round trip: %s", f)
	}
	for i, f := range unknownOps {
		if i == 10 {
			t.Errorf("... and %d more unknown operators", len(unknownOps)-10)
			break
		}
		t.Errorf("operator matches nothing in doXMath: %s", f)
	}

	// A floor, not a measurement: the corpus writes roughly 16,500 of these,
	// and anything far below means the walk stopped early.
	if parsed < 15000 {
		t.Fatalf("read %d amounts, want at least 15000 -- the walk is broken", parsed)
	}
	t.Logf("parsed %d amounts: %d heads, %d operators, %d count heads",
		parsed, len(heads), len(operators), len(countHeads))

	var lines []string
	for _, name := range sortedKeys(heads) {
		lines = append(lines, fmt.Sprintf("head\t%s", name))
	}
	for _, name := range sortedKeys(contexts) {
		lines = append(lines, fmt.Sprintf("context\t%s\t%d", name, contexts[name]))
	}
	for _, name := range sortedKeys(operators) {
		lines = append(lines, fmt.Sprintf("operator\t%s\t%d", name, operators[name]))
	}
	for _, name := range sortedKeys(countHeads) {
		lines = append(lines, fmt.Sprintf("countHead\t%s", name))
	}
	compareGolden(t, "testdata/expressions.golden", lines)
}

// forEachAmount hands fn every amount expression on every face, from the SVar
// bodies that are measurements and from the `Count$` values inside params.
func forEachAmount(t *testing.T, fn func(card, amount string)) {
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

func eachFace(face *carddb.Face, card string, fn func(card, amount string)) {
	for _, name := range face.SVars.Names() {
		value, _ := face.SVars.Get(name)
		// A body leading with a record key is an ability, not an amount.
		if params := vocab.SplitParams(value); len(params) > 0 && isRecordKey(params[0].Key) {
			continue
		}
		fn(card, value)
	}

	lines := make([]string, 0, 16)
	lines = append(lines, face.Abilities...)
	lines = append(lines, face.Triggers...)
	lines = append(lines, face.Statics...)
	lines = append(lines, face.Replacements...)
	for _, line := range lines {
		for _, p := range vocab.SplitParams(line) {
			if strings.HasPrefix(p.Value, "Count$") {
				fn(card, p.Value)
			}
		}
	}

	for _, variant := range face.Variants {
		eachFace(variant, card, fn)
	}
}

func isRecordKey(key string) bool {
	switch strings.ToLower(key) {
	case "sp", "ab", "db", "st", "re", "mode", "event":
		return true
	}
	return false
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
	// crucible/internal/expr/<file> -> repository root.
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
