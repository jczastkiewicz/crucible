package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
)

// TestChooseNumberEffectRecordsChosenNumber proves the open shape: a
// number within [Min$, Max$] is recorded on the host.
func TestChooseNumberEffectRecordsChosenNumber(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGame(t)
	c := engine.NewScriptedController()
	c.QueueNumberChoice(3)
	host, err := castETBChain(t, g, p, etbChainDef(t, "Test ChooseNumber", "DB$ ChooseNumber | Defined$ You | Min$ 1 | Max$ 5"), c)
	if err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if n, ok := g.Card(host).Memory.ChosenNumber(); !ok || n != 3 {
		t.Errorf("ChosenNumber = %d, %v, want 3, true", n, ok)
	}
}

// TestChooseNumberEffectRejectsOutOfRange proves an answer above Max$ is an
// error (GO-7).
func TestChooseNumberEffectRejectsOutOfRange(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGame(t)
	c := engine.NewScriptedController()
	c.QueueNumberChoice(7)
	def := etbChainDef(t, "Test ChooseNumber Range", "DB$ ChooseNumber | Defined$ You | Max$ 5")
	if _, err := castETBChain(t, g, p, def, c); err == nil {
		t.Fatal("ResolveStack succeeded, want an error for a number above Max$")
	}
}

// TestChooseNumberEffectSecretlyRunsHighestAndNotLowest proves the Secretly$
// reveal (Wheel of Misfortune): every player chooses, Highest$ runs for the
// highest chooser with their number, NotLowest$ for everyone but the lowest.
func TestChooseNumberEffectSecretlyRunsHighestAndNotLowest(t *testing.T) {
	t.Parallel()

	g, p, other := newTwoPlayerGame(t)
	c := engine.NewScriptedController()
	c.QueueNumberChoice(5)
	c.QueueNumberChoice(2)
	if _, err := castETBChain(t, g, p, etbChainDef(t, "Test Wheel",
		"DB$ ChooseNumber | Defined$ Player | Secretly$ True | Highest$ DBHurt | NotLowest$ DBGain | Lowest$ DBHeal",
		"DBHurt", "DB$ DealDamage | Defined$ Remembered | NumDmg$ Count$ChosenNumber",
		"DBGain", "DB$ GainLife | Defined$ Remembered | LifeAmount$ 1",
		"DBHeal", "DB$ GainLife | Defined$ Remembered | LifeAmount$ 10"), c); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if got := g.Player(p).Life; got != 16 {
		t.Errorf("highest chooser life = %d, want 20 - 5 + 1", got)
	}
	if got := g.Player(other).Life; got != 30 {
		t.Errorf("lowest chooser life = %d, want 20 + 10", got)
	}
}

// TestChooseNumberEffectSecretlyMatchedWhenAllTie proves MatchedAbility$
// runs once when every chooser picked the highest number, Unmatched$ else.
func TestChooseNumberEffectSecretlyMatchedWhenAllTie(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		a, b, want int
	}{{3, 3, 25}, {3, 1, 17}} {
		g, p, _ := newTwoPlayerGame(t)
		c := engine.NewScriptedController()
		c.QueueNumberChoice(tc.a)
		c.QueueNumberChoice(tc.b)
		if _, err := castETBChain(t, g, p, etbChainDef(t, "Test Matched",
			"DB$ ChooseNumber | Defined$ Player | Secretly$ True | MatchedAbility$ DBUp | UnmatchedAbility$ DBDown",
			"DBUp", "DB$ GainLife | Defined$ You | LifeAmount$ 5",
			"DBDown", "DB$ LoseLife | Defined$ You | LifeAmount$ 3"), c); err != nil {
			t.Fatalf("ResolveStack: %v", err)
		}
		if got := g.Player(p).Life; got != tc.want {
			t.Errorf("choices %d,%d: life = %d, want %d", tc.a, tc.b, got, tc.want)
		}
	}
}

// TestChooseNumberEffectRejectsUnresolvedParam proves Guesser$ fails closed.
func TestChooseNumberEffectRejectsUnresolvedParam(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGame(t)
	c := engine.NewScriptedController()
	def := etbChainDef(t, "Test ChooseNumber Secret", "DB$ ChooseNumber | Defined$ You | Guesser$ Opponent")
	if _, err := castETBChain(t, g, p, def, c); err == nil {
		t.Fatal("ResolveStack succeeded, want an error for unresolved Guesser$")
	}
}

// TestChooseNumberEffectRejectsBadBounds proves an unresolvable Min$/Max$
// or Defined$ fails closed.
func TestChooseNumberEffectRejectsBadBounds(t *testing.T) {
	t.Parallel()

	for _, line := range []string{
		"DB$ ChooseNumber | Defined$ You | Min$ Bogus",
		"DB$ ChooseNumber | Defined$ You | Max$ Bogus",
		"DB$ ChooseNumber | Defined$ TriggeredPlayer",
	} {
		g, p, _ := newTwoPlayerGame(t)
		c := engine.NewScriptedController()
		if _, err := castETBChain(t, g, p, etbChainDef(t, "Test ChooseNumber Bounds", line), c); err == nil {
			t.Errorf("%q: ResolveStack succeeded, want an error", line)
		}
	}
}
