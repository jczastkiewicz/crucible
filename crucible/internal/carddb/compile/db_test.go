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
