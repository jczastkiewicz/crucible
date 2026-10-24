package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
)

// TestExploreEffectLandGoesToHand proves CR 701.44a's land branch: the
// revealed land goes to hand and the explorer gets no counter.
func TestExploreEffectLandGoesToHand(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGame(t)
	land := g.NewCard(landDef(t, "Test Land", "Basic Land Plains"), p, engine.Library)

	c := engine.NewScriptedController()
	host, err := castETBChain(t, g, p, etbChainDef(t, "Test Explore Land", "DB$ Explore"), c)
	if err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if z := g.Card(land).Zone; z != engine.Hand {
		t.Errorf("land zone = %v, want Hand", z)
	}
	if n := g.Card(host).Counters.Count(engine.P1P1); n != 0 {
		t.Errorf("explorer +1/+1 counters = %d, want 0", n)
	}
}

// TestExploreEffectNonlandCounterAndChoice proves the nonland branch: a
// +1/+1 counter either way, and the revealed card goes to the graveyard
// only when the controller says so.
func TestExploreEffectNonlandCounterAndChoice(t *testing.T) {
	t.Parallel()

	for _, toGraveyard := range []bool{true, false} {
		g, p, _ := newTwoPlayerGame(t)
		top := libraryCards(t, g, p, 1)[0]
		c := engine.NewScriptedController()
		c.QueueConfirmEffect(toGraveyard)
		host, err := castETBChain(t, g, p, etbChainDef(t, "Test Explore", "DB$ Explore"), c)
		if err != nil {
			t.Fatalf("ResolveStack: %v", err)
		}
		want := engine.Library
		if toGraveyard {
			want = engine.Graveyard
		}
		if z := g.Card(top).Zone; z != want {
			t.Errorf("toGraveyard=%v: revealed zone = %v, want %v", toGraveyard, z, want)
		}
		if n := g.Card(host).Counters.Count(engine.P1P1); n != 1 {
			t.Errorf("toGraveyard=%v: explorer counters = %d, want 1", toGraveyard, n)
		}
	}
}

// Player.getNumExploredThisTurn counts each explore (ExploreEffect.java:103),
// the amount Herald's Reveille's Branch reads.
func TestExploreCountsTowardExploredThisTurn(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGame(t)
	g.NewCard(landDef(t, "Test Land", "Basic Land Plains"), p, engine.Library)
	before := g.Player(p).Life
	if _, err := castETBChain(t, g, p, etbChainDef(t, "Test Explore Count",
		"DB$ Explore | Num$ 1 | SubAbility$ DBGain",
		"DBGain", "DB$ GainLife | Defined$ You | LifeAmount$ X",
		"X", "PlayerCountPropertyYou$ExploredThisTurn"), engine.NewScriptedController()); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if got := g.Player(p).Life - before; got != 1 {
		t.Errorf("explored this turn = %d, want 1", got)
	}
}

// A Branch compares against an inline expression operand (Prisoner's Dilemma's
// EQPlayerCountOpponents$Amount): a '$' past the first character is the
// expression itself, not an SVar name (AbilityUtils.java:402-404).
func TestBranchCompareReadsAnInlineExpression(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGame(t)
	before := g.Player(p).Life
	resolveLine(t, g, p, engine.NewScriptedController(),
		"DB$ Branch | BranchConditionSVar$ X | BranchConditionSVarCompare$ EQPlayerCountOpponents$Amount | TrueSubAbility$ T | FalseSubAbility$ F",
		"X", "PlayerCountOpponents$Amount",
		"T", "DB$ GainLife | Defined$ You | LifeAmount$ 5",
		"F", "DB$ GainLife | Defined$ You | LifeAmount$ 1")
	if got := g.Player(p).Life - before; got != 5 {
		t.Errorf("gained %d, want 5 (1 opponent equals the count)", got)
	}
}
