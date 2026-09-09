package keyword_test

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
	"github.com/jczastkiewicz/crucible/internal/cardtype"
	"github.com/jczastkiewicz/crucible/internal/cost"
	"github.com/jczastkiewicz/crucible/internal/expr"
	"github.com/jczastkiewicz/crucible/internal/keyword"
)

var update = flag.Bool("update", false, "rewrite golden files")

// TestCorpusKeywords parses every `K:` line the corpus writes.
//
// The golden holds two lists that matter to M3. The keywords Forge defines and
// the corpus uses, by argument shape, are what a compiler owes an
// implementation. The heads Forge does **not** define are the pseudo-keywords
// the card factory handles instead -- `etbCounter`, `ETBReplacement`,
// `Chapter` -- plus the lines that are simply rules text, and knowing which is
// which is the difference between a vocabulary gate that can close and one
// that cannot.
//
// Regenerate with:
//
//	go test ./internal/keyword -run TestCorpusKeywords -update
func TestCorpusKeywords(t *testing.T) {
	t.Parallel()

	var (
		parsed       int
		defined      = map[string]int{}
		undefined    = map[string]int{}
		textKeywords = map[string]int{}
		byKind       = map[keyword.Kind]int{}
		badAmount    []string
		roundTrip    []string
	)
	forEachKeyword(t, func(card, line string) {
		parsed++
		k := keyword.Parse(line)
		if got := k.String(); got != line {
			roundTrip = append(roundTrip, fmt.Sprintf("%s: %q became %q", card, line, got))
		}
		byKind[k.Kind()]++

		if k.Entry == nil {
			// Two different things end up here. A single-token head is a
			// pseudo-keyword the card factory handles -- `etbCounter`,
			// `ETBReplacement`, `Chapter` -- and belongs in the vocabulary. A
			// head with a space is a whole sentence of rules text carried on a
			// K: line, which is card text rather than vocabulary and would move
			// this golden every time a card is added.
			if strings.Contains(k.Name, " ") {
				textKeywords[k.Name]++
			} else {
				undefined[k.Name]++
			}
			return
		}
		defined[k.Entry.Name]++

		// Where Java says the argument is a number, it must read as one --
		// KeywordWithAmount calls Integer.parseInt on it, and an `X` is the
		// documented alternative.
		if k.Kind() == keyword.Amount && k.Details != "" {
			amount := strings.SplitN(k.Details, ":", 2)[0]
			if parsedAmount := expr.Parse(amount); parsedAmount.Kind != expr.Literal && amount != "X" &&
				!strings.HasPrefix(amount, "X") {
				badAmount = append(badAmount, fmt.Sprintf("%s: %q has the amount %q", card, line, amount))
			}
		}
		// Where it says the argument is a cost, it must parse as one.
		if k.Kind() == keyword.Cost && k.Details != "" {
			cost.Parse(strings.SplitN(k.Details, "|", 2)[0])
		}
	})

	report(t, "round trip", roundTrip)
	report(t, "amount keyword with a non-numeric argument", badAmount)

	if parsed < 15000 {
		t.Fatalf("read %d keywords, want at least 15000 -- the walk is broken", parsed)
	}
	textUses := 0
	for _, n := range textKeywords {
		textUses += n
	}
	t.Logf("parsed %d keywords: %d defined by Forge, %d pseudo-keywords, %d lines of rules text",
		parsed, len(defined), len(undefined), textUses)

	var lines []string
	for kind := keyword.Unknown; kind <= keyword.Special; kind++ {
		lines = append(lines, fmt.Sprintf("kind\t%s\t%d", kind, byKind[kind]))
	}
	for _, name := range sortedKeys(defined) {
		lines = append(lines, fmt.Sprintf("used\t%s\t%s", name, keyword.Lookup(name).Kind))
	}
	for _, name := range sortedKeys(undefined) {
		lines = append(lines, fmt.Sprintf("pseudo\t%s\t%d", name, undefined[name]))
	}
	// Counted, not listed: the text is the card's, not the language's.
	lines = append(lines, fmt.Sprintf("textKeywords\t%d distinct", len(textKeywords)))
	compareGolden(t, "testdata/keywords.golden", lines)
}

func report(t *testing.T, what string, failures []string) {
	t.Helper()

	sort.Strings(failures)
	for i, f := range failures {
		if i == 10 {
			t.Errorf("%s: ... and %d more", what, len(failures)-10)
			break
		}
		t.Errorf("%s: %s", what, f)
	}
}

func forEachKeyword(t *testing.T, fn func(card, line string)) {
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

func eachFace(face *carddb.Face, card string, fn func(card, line string)) {
	for _, line := range face.Keywords {
		fn(card, line)
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
	// crucible/internal/keyword/<file> -> repository root.
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
