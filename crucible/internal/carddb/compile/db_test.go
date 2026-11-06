package compile_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/carddb/compile"
)

func TestNewDB(t *testing.T) {
	t.Parallel()

	bears := compileScript(t, "Name:Grizzly Bears\nManaCost:1 G\nTypes:Creature Bear\nPT:2/2\nOracle:\n")
	db := compile.NewDB(map[string]*compile.Card{"Grizzly Bears": bears})

	if db.Len() != 1 {
		t.Errorf("Len %d, want 1", db.Len())
	}
	got, ok := db.Card("Grizzly Bears")
	if !ok || got != bears {
		t.Error("Card did not return the card NewDB was given")
	}
	if _, ok := db.Card("Nonexistent"); ok {
		t.Error("Card found a name that was never added")
	}
	if names := db.Names(); len(names) != 1 || names[0] != "Grizzly Bears" {
		t.Errorf("Names %v, want [Grizzly Bears]", names)
	}
}

// Resolve reads a decklist's spelling: any case, accents folded, and a split
// card by both halves the way Forge's .dck files write it ("Wear // Tear"),
// though Card keys it by its first half.
func TestResolveReadsDecklistSpellings(t *testing.T) {
	t.Parallel()

	wear := compileScript(t, "Name:Wear\nManaCost:1 R\nTypes:Instant\nAlternateMode:Split\nOracle:\n\nALTERNATE\n\n"+
		"Name:Tear\nManaCost:W\nTypes:Instant\nOracle:\n")
	troll := compileScript(t, "Name:Troll of Khazad-dûm\nManaCost:5 B\nTypes:Creature Troll\nPT:6/5\nOracle:\n")
	db := compile.NewDB(map[string]*compile.Card{"Wear": wear, "Troll of Khazad-dûm": troll})

	for _, tc := range []struct{ in, want string }{
		{"Wear // Tear", "Wear"},
		{"wear // tear", "Wear"},
		{"Wear", "Wear"},
		{"Troll of Khazad-dum", "Troll of Khazad-dûm"},
		{"TROLL OF KHAZAD-DÛM", "Troll of Khazad-dûm"},
	} {
		if got, ok := db.Resolve(tc.in); !ok || got != tc.want {
			t.Errorf("Resolve(%q) = %q, %v; want %q", tc.in, got, ok, tc.want)
		}
	}
	for _, in := range []string{"Tear", "Wear // Fire", "Grizzly Bears"} {
		if got, ok := db.Resolve(in); ok {
			t.Errorf("Resolve(%q) = %q, want no card", in, got)
		}
	}
}
