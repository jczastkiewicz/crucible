package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
)

// Ashiok, Wicked Manipulator: "If you would pay life while your library has at
// least that many cards in it, exile that many cards from the top of your
// library instead" (Event$ PayLife | Amount$ LEY, ReplacePayLife.java:47-55).
// With 3 cards in the library a 2-life cost exiles 2 cards; with 1 card the
// life is paid.
func TestAshiokExilesLibraryCardsInsteadOfPayingLife(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		library  int
		wantLife int
		wantLib  int
	}{
		{3, 20, 1},
		{1, 18, 1},
	} {
		g, p, _ := newTwoPlayerGameOn(t, scenarioDB(t))
		g.SetTurnState(1, p, engine.Main1)
		g.NewCard(corpusCard(t, "Ashiok, Wicked Manipulator"), p, engine.Battlefield)
		lib := libraryCards(t, g, p, tc.library)
		payer := g.NewCard(creatureDefWithAbility(t, "Life Payer", "AB$ Draw | Cost$ PayLife<2> | Defined$ You | NumCards$ 0"), p, engine.Battlefield)
		c := engine.NewScriptedController()
		if tc.library == 3 {
			c.QueueCardOrder(lib[:2])
		}
		if !g.ActivateAbility(p, payer, 0, c) {
			t.Fatalf("library %d: activation refused (%v)", tc.library, g.TakePendingError())
		}
		if err := g.TakePendingError(); err != nil {
			t.Fatalf("library %d: %v", tc.library, err)
		}
		if got := g.Player(p).Life; got != tc.wantLife {
			t.Errorf("library %d: life %d, want %d", tc.library, got, tc.wantLife)
		}
		if got := g.Zone(engine.Library, p).Cards(); len(got) != tc.wantLib || got[0] != lib[len(lib)-1] {
			t.Errorf("library %d: library %v, want only the bottom card %v", tc.library, got, lib[len(lib)-1])
		}
	}
}
