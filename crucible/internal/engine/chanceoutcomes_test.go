package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
	"github.com/jczastkiewicz/crucible/pkg/javarand"
)

// fixedChance answers coin flips and die rolls from queues, as a replay's
// recorded outcomes do.
type fixedChance struct {
	coins []bool
	faces []int
}

func (f *fixedChance) Coin(engine.PlayerID) (bool, bool) {
	if len(f.coins) == 0 {
		return false, false
	}
	heads := f.coins[0]
	f.coins = f.coins[1:]
	return heads, true
}

func (f *fixedChance) Die(_ engine.PlayerID, _ int) (int, bool) {
	if len(f.faces) == 0 {
		return 0, false
	}
	face := f.faces[0]
	f.faces = f.faces[1:]
	return face, true
}

// A game given a Chance flips and rolls what it says, not its own stream
// (ADR-0059's replay mode); a face the die does not have rolls with the
// stream.
func TestGameTakesCoinsAndDiceFromItsChance(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGame(t)
	g.SetChance(&fixedChance{coins: []bool{true, false, true}})
	resolveLine(t, g, p, engine.NewScriptedController(),
		"DB$ FlipCoin | NoCall$ True | Amount$ 3 | HeadsSubAbility$ DBHeads | TailsSubAbility$ DBTails",
		"DBHeads", "DB$ GainLife | Defined$ You | LifeAmount$ X",
		"DBTails", "DB$ LoseLife | Defined$ You | LifeAmount$ X")
	if got := g.Player(p).Life; got != 21 {
		t.Errorf("life = %d, want 21 (two heads, one tail)", got)
	}

	natural := int(javarand.New(1).Int32n(6)) + 1
	for _, tc := range []struct {
		name string
		face int
		want int
	}{
		{"recorded face", 5, 5},
		{"face the die lacks", 9, natural},
	} {
		g, p, _ := newTwoPlayerGame(t)
		g.SetChance(&fixedChance{faces: []int{tc.face}})
		resolveLine(t, g, p, engine.NewScriptedController(),
			"DB$ RollDice | Sides$ 6 | ResultSVar$ Result | SubAbility$ DBGain",
			"DBGain", "DB$ GainLife | Defined$ You | LifeAmount$ Result")
		if got := g.Player(p).Life - 20; got != tc.want {
			t.Errorf("%s: gained %d, want %d", tc.name, got, tc.want)
		}
	}
}
