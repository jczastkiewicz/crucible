package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
)

// TestRepeatEffectStopsAtMaxRepeat proves RepeatSubAbility$ runs once, then
// again while RepeatOptional$ is accepted, capped by MaxRepeat$.
func TestRepeatEffectStopsAtMaxRepeat(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGame(t)
	c := engine.NewScriptedController()
	c.QueueConfirmEffect(true)
	c.QueueConfirmEffect(true)
	def := etbChainDef(t, "Test Repeat", "DB$ Repeat | RepeatSubAbility$ DBGain | RepeatOptional$ True | MaxRepeat$ 3",
		"DBGain", "DB$ GainLife | Defined$ You | LifeAmount$ 1")
	if _, err := castETBChain(t, g, p, def, c); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if got := g.Player(p).Life; got != 23 {
		t.Errorf("life = %d, want 23", got)
	}
}

// TestRepeatEffectDeclinedAfterFirst proves the first resolution always
// happens and a declined RepeatOptional$ stops there.
func TestRepeatEffectDeclinedAfterFirst(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGame(t)
	c := engine.NewScriptedController()
	c.QueueConfirmEffect(false)
	def := etbChainDef(t, "Test Repeat Once", "DB$ Repeat | RepeatSubAbility$ DBGain | RepeatOptional$ True",
		"DBGain", "DB$ GainLife | Defined$ You | LifeAmount$ 1")
	if _, err := castETBChain(t, g, p, def, c); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if got := g.Player(p).Life; got != 21 {
		t.Errorf("life = %d, want 21", got)
	}
}

// TestRepeatEffectCheckSVarStopsLoop proves RepeatCheckSVar$ ends the loop
// when its compare fails, and an unresolvable one fails closed.
func TestRepeatEffectCheckSVarStopsLoop(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGame(t)
	c := engine.NewScriptedController()
	def := etbChainDef(t, "Test Repeat SVar", "DB$ Repeat | RepeatSubAbility$ DBGain | RepeatCheckSVar$ 0 | RepeatSVarCompare$ GE1",
		"DBGain", "DB$ GainLife | Defined$ You | LifeAmount$ 1")
	if _, err := castETBChain(t, g, p, def, c); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if got := g.Player(p).Life; got != 21 {
		t.Errorf("life = %d, want 21", got)
	}
	g2, p2, _ := newTwoPlayerGame(t)
	bad := etbChainDef(t, "Test Repeat Bad", "DB$ Repeat | RepeatSubAbility$ DBGain | RepeatCheckSVar$ Bogus",
		"DBGain", "DB$ GainLife | Defined$ You | LifeAmount$ 1")
	if _, err := castETBChain(t, g2, p2, bad, engine.NewScriptedController()); err == nil {
		t.Fatal("ResolveStack succeeded, want an error for an unresolvable RepeatCheckSVar$")
	}
}
