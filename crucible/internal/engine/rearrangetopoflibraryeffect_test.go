package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
)

// TestRearrangeTopOfLibraryEffectPutsBackInOrder proves Java's
// moveToLibrary(next, 0) loop: each card in the answered order goes on top
// in turn, so the last one ends up on top.
func TestRearrangeTopOfLibraryEffectPutsBackInOrder(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGame(t)
	lib := libraryCards(t, g, p, 4)

	c := engine.NewScriptedController()
	c.QueueCardOrder([]engine.CardID{lib[2], lib[0], lib[1]})
	if _, err := castETBChain(t, g, p, etbChainDef(t, "Test Rearrange", "DB$ RearrangeTopOfLibrary | Defined$ You | NumCards$ 3"), c); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	got := g.Zone(engine.Library, p).Cards()
	want := []engine.CardID{lib[1], lib[0], lib[2], lib[3]}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("library = %v, want %v", got, want)
		}
	}
}

// TestRearrangeTopOfLibraryEffectRejectsBadAnswer proves an answer that is
// not a permutation of the top cards is an error (GO-7).
func TestRearrangeTopOfLibraryEffectRejectsBadAnswer(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGame(t)
	lib := libraryCards(t, g, p, 3)

	c := engine.NewScriptedController()
	c.QueueCardOrder([]engine.CardID{lib[0]})
	if _, err := castETBChain(t, g, p, etbChainDef(t, "Test Rearrange Bad", "DB$ RearrangeTopOfLibrary | Defined$ You | NumCards$ 2 | MayShuffle$ True"), c); err == nil {
		t.Fatal("ResolveStack succeeded, want an error for a short ordering")
	}
}
