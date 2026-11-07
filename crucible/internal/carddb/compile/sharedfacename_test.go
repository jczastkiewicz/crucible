package compile_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/jczastkiewicz/crucible/internal/carddb/compile"
)

// TestSplitCardSharingAFaceKeepsBothCards pins LoadDB's key for two cards
// whose first faces share a name (Bind and Bind // Liberate, whose Bind is
// CopyFaceFrom:Bind): the card named by that face keeps it, the Split card
// is held under its full name, and both are listed, in either walk order.
func TestSplitCardSharingAFaceKeepsBothCards(t *testing.T) {
	t.Parallel()

	standalone := "Name:Bind\nManaCost:1 G\nTypes:Instant\nA:SP$ Draw | SpellDescription$ Draw a card.\n"
	split := "Name:Bind\nManaCost:1 G\nTypes:Instant\nA:SP$ Draw | SpellDescription$ Draw a card.\nAlternateMode:Split\nALTERNATE\nName:Liberate\nManaCost:1 W\nTypes:Instant\nA:SP$ GainLife | LifeAmount$ 1 | SpellDescription$ Gain 1 life.\n"
	typeList := filepath.Join(repoRoot(t), "forge-gui", "res", "lists", "TypeLists.txt")
	for name, files := range map[string][2]string{
		"StandaloneFirst": {"a_bind.txt", "b_bind_liberate.txt"},
		"SplitFirst":      {"b_bind.txt", "a_bind_liberate.txt"},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			dir := filepath.Join(t.TempDir(), "b")
			if err := os.MkdirAll(dir, 0o755); err != nil {
				t.Fatal(err)
			}
			for i, text := range []string{standalone, split} {
				if err := os.WriteFile(filepath.Join(dir, files[i]), []byte(text), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			db, err := compile.LoadDB(filepath.Dir(dir), typeList)
			if err != nil {
				t.Fatalf("LoadDB: %v", err)
			}
			if db.Len() != 2 || len(db.Names()) != 2 {
				t.Fatalf("Len = %d, Names = %v, want both cards", db.Len(), db.Names())
			}
			if c, ok := db.Card("Bind"); !ok || c.Faces[1].Name != "" {
				t.Errorf("Card(Bind) = %v, %v, want the standalone Bind", c, ok)
			}
			if c, ok := db.Card("Bind // Liberate"); !ok || c.Faces[1].Name != "Liberate" {
				t.Errorf("Card(Bind // Liberate) = %v, %v, want the Split card", c, ok)
			}
			if n, ok := db.Resolve("bind // liberate"); !ok || n != "Bind // Liberate" {
				t.Errorf("Resolve(bind // liberate) = %q, %v", n, ok)
			}
		})
	}
}
