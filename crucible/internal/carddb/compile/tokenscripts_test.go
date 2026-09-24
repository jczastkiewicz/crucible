package compile_test

import (
	"path/filepath"
	"testing"

	"github.com/jczastkiewicz/crucible/internal/carddb/compile"
)

// TestTokenScriptsCompile is the token-script half of the P2 gate: every
// res/tokenscripts file compiles, keyed by its file name, so a TokenScript$
// naming any of them resolves (PORT-8).
func TestTokenScriptsCompile(t *testing.T) {
	t.Parallel()

	root := filepath.Join(repoRoot(t), "forge-gui", "res", "tokenscripts")
	typeList := filepath.Join(repoRoot(t), "forge-gui", "res", "lists", "TypeLists.txt")
	tokens, err := compile.LoadTokenScripts(root, typeList)
	if err != nil {
		t.Fatalf("LoadTokenScripts: %v", err)
	}
	if len(tokens) < 800 {
		t.Fatalf("compiled %d token scripts, want at least 800", len(tokens))
	}
	db := compile.NewDB(nil).WithTokens(tokens)
	clue, ok := db.Token("c_a_clue_draw")
	if !ok {
		t.Fatal(`Token("c_a_clue_draw") missing`)
	}
	if got := clue.Name; got != "Clue Token" {
		t.Errorf("c_a_clue_draw name = %q, want %q", got, "Clue Token")
	}
	var nilDB *compile.DB
	if _, ok := nilDB.Token("c_a_clue_draw"); ok {
		t.Error("a nil DB reported a token")
	}
}
