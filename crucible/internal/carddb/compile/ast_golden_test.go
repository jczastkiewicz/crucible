package compile_test

import (
	"bytes"
	"flag"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/jczastkiewicz/crucible/internal/carddb/compile"
)

var update = flag.Bool("update", false, "rewrite the golden files")

// shapePins are the cards whose full canonical AST is committed, chosen to
// cover every record type and every way one ability names another. A hash
// golden says which card changed; these say what a change looks like, which is
// the difference between a reviewable diff and a wall of hex.
var shapePins = []string{
	"ancestral_recall",
	"orochi_hatchery",
	"beorns_hospitality",
	"leader_super_genius",
	"typhoid_mary_fractured",
	"invasion_of_arcavios_invocation_of_the_founders",
}

// TestCorpusAST is M3's golden AST diff.
//
// The compiler is rewritten throughout the port and almost every rewrite is
// meant to change nothing. Without a golden, "nothing" is an assertion; with
// one, a change to any of 33,689 compiled cards is a diff someone has to look
// at and accept. Regenerate deliberately:
//
//	go test ./internal/carddb/compile -run TestCorpusAST -update
//
// and read the diff before committing it. A golden updated without being read
// is worse than no golden, because it converts an unexplained change into an
// approved one.
func TestCorpusAST(t *testing.T) {
	t.Parallel()

	cards := parseCorpus(t)
	var (
		lines  []string
		shapes = map[string]string{}
		pinned = map[string]bool{}
	)
	for _, name := range shapePins {
		pinned[name] = true
	}

	for _, card := range cards {
		out, err := compile.Compile(card)
		if err != nil {
			// TestCorpusCompiles owns compile failures and reports them with
			// the card and the reason. Here they would only be noise.
			continue
		}
		lines = append(lines, out.Filename+"\t"+compile.Fingerprint(out))

		if pinned[out.Filename] {
			var buf bytes.Buffer
			if err := compile.WriteCanonical(&buf, out); err != nil {
				t.Fatalf("canonical %s: %v", out.Filename, err)
			}
			shapes[out.Filename] = buf.String()
		}
	}
	sort.Strings(lines)

	var shaped strings.Builder
	for _, name := range shapePins {
		text, ok := shapes[name]
		if !ok {
			t.Errorf("pinned shape %q is not in the corpus; pick another card or drop the pin", name)
			continue
		}
		shaped.WriteString("card " + name + "\n")
		shaped.WriteString(text)
		shaped.WriteString("\n")
	}

	compareGolden(t, filepath.Join("testdata", "ast.golden"), strings.Join(lines, "\n")+"\n")
	compareGolden(t, filepath.Join("testdata", "ast-shapes.golden"), shaped.String())
}

// compareGolden writes the file under -update and otherwise reports the first
// differing line. The whole diff would be 33,689 lines wide on a bad day, so
// the failure names a place to look rather than reprinting the corpus.
func compareGolden(t *testing.T, path, got string) {
	t.Helper()

	if *update {
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
		t.Logf("wrote %s", path)
		return
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v (regenerate with -update)", path, err)
	}
	want := string(raw)
	if got == want {
		return
	}

	gotLines, wantLines := strings.Split(got, "\n"), strings.Split(want, "\n")
	for i := 0; i < len(gotLines) && i < len(wantLines); i++ {
		if gotLines[i] != wantLines[i] {
			t.Fatalf("%s line %d:\n got %s\nwant %s\n(%d lines now, %d in the golden; regenerate with -update and read the diff)",
				path, i+1, gotLines[i], wantLines[i], len(gotLines), len(wantLines))
		}
	}
	t.Fatalf("%s has %d lines, generated %d (regenerate with -update and read the diff)",
		path, len(wantLines), len(gotLines))
}
