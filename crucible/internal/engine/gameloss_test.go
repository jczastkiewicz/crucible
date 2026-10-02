package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
	"github.com/jczastkiewicz/crucible/pkg/javarand"
)

// CR 103.3: every player starts the game at 20 life.
func TestNewGameStartsEveryPlayerAtTwentyLife(t *testing.T) {
	t.Parallel()

	g := engine.NewGame(scenarioDB(t), javarand.New(1), []string{"a", "b", "c"})
	for _, pid := range g.Players() {
		if got := g.Player(pid).Life; got != 20 {
			t.Errorf("player %d starts at %d life, want 20", pid, got)
		}
	}
}

// Platinum Angel's second line (Event$ GameWin, ValidPlayer$ Opponent, Layer$
// CantHappen): the opposing player's "wins the game" effect does nothing
// (Player.altWinBySpellEffect), and the game is still on.
func TestCantWinStopsAWinsGameEffect(t *testing.T) {
	t.Parallel()

	g, p, other := newTwoPlayerGameOn(t, scenarioDB(t))
	g.NewCard(corpusCard(t, "Platinum Angel"), other, engine.Battlefield)
	if err := resolveWith(t, g, p, engine.NewScriptedController(), "DB$ WinsGame | Defined$ You"); err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if g.Player(p).Won {
		t.Error("the player won the game against a Platinum Angel")
	}
	// An unaffected player still wins.
	if err := resolveWith(t, g, other, engine.NewScriptedController(), "DB$ WinsGame | Defined$ You"); err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if !g.Player(other).Won {
		t.Error("the Angel's controller did not win")
	}
}

// A LosesGame effect is a SpellEffect loss, which "can't lose" stops; a
// concession is not.
func TestCantLoseStopsALosesGameEffectButNotAConcession(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGameOn(t, scenarioDB(t))
	g.NewCard(corpusCard(t, "Platinum Angel"), p, engine.Battlefield)
	if err := resolveWith(t, g, p, engine.NewScriptedController(), "DB$ LosesGame | Defined$ You"); err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if g.Player(p).Lost {
		t.Error("the player lost to an effect despite Platinum Angel")
	}
	g.Concede(p)
	if !g.Player(p).Lost {
		t.Error("a concession did not lose the game")
	}
}
