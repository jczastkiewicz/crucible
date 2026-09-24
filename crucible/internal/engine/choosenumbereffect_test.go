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

// TestChooseNumberEffectRejectsUnresolvedParam proves Secretly$ fails closed.
func TestChooseNumberEffectRejectsUnresolvedParam(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGame(t)
	c := engine.NewScriptedController()
	def := etbChainDef(t, "Test ChooseNumber Secret", "DB$ ChooseNumber | Defined$ You | Secretly$ True")
	if _, err := castETBChain(t, g, p, def, c); err == nil {
		t.Fatal("ResolveStack succeeded, want an error for unresolved Secretly$")
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
