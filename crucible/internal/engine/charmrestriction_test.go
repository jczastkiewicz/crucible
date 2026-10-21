package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
)

// ChoiceRestriction$ removes a mode the Charm already chose (this turn, or
// this game) from the modes on offer; ThisTurn gives it back as the next turn
// begins, ThisGame never does, and a Charm left with too few modes is not
// activated at all.
func TestCharmChoiceRestrictionHidesChosenModes(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		restriction string
		// life gained by three activations that always take the first mode
		// on offer: two in turn 1, one in turn 2.
		want   [3]int
		third  bool
		reason string
	}{
		{"ThisTurn", [3]int{1, 2, 1}, true, "the first mode is back in the next turn"},
		{"ThisGame", [3]int{1, 2, 0}, false, "both modes are spent for the game"},
	} {
		t.Run(tc.restriction, func(t *testing.T) {
			t.Parallel()
			g, p, _ := newTwoPlayerGame(t)
			engineCard := g.NewCard(scriptDef(t, "Restricted Charm", "Artifact",
				"A:AB$ Charm | Cost$ 0 | Choices$ DBA,DBB | ChoiceRestriction$ "+tc.restriction,
				"SVar:DBA:DB$ GainLife | Defined$ You | LifeAmount$ 1 | SpellDescription$ one",
				"SVar:DBB:DB$ GainLife | Defined$ You | LifeAmount$ 2 | SpellDescription$ two"), p, engine.Battlefield)
			c := engine.NewScriptedController()
			activate := func() (int, bool) {
				before := g.Player(p).Life
				c.QueueModeChoice([]int{0})
				if !g.ActivateAbility(p, engineCard, 0, c) {
					return 0, false
				}
				if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
					t.Fatal(err)
				}
				return g.Player(p).Life - before, true
			}
			for i := 0; i < 2; i++ {
				if got, ok := activate(); !ok || got != tc.want[i] {
					t.Fatalf("activation %d gained %d (activated %v), want %d", i+1, got, ok, tc.want[i])
				}
			}
			if _, ok := activate(); ok {
				t.Fatal("a third activation in the same turn found a mode left")
			}
			advanceToTurn(t, g, g.Turn()+1)
			got, ok := activate()
			if ok != tc.third || got != tc.want[2] {
				t.Errorf("next turn: gained %d (activated %v), want %d (%v): %s", got, ok, tc.want[2], tc.third, tc.reason)
			}
		})
	}
}

// YourLastCombat hides what the Charm chose in its controller's previous
// combat; the modes chosen in a combat are only remembered once that combat
// ends, and only for its controller's turn.
func TestCharmChoiceRestrictionYourLastCombat(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGame(t)
	charm := g.NewCard(scriptDef(t, "Combat Charm", "Artifact",
		"A:AB$ Charm | Cost$ 0 | Choices$ DBA,DBB | ChoiceRestriction$ YourLastCombat",
		"SVar:DBA:DB$ GainLife | Defined$ You | LifeAmount$ 1 | SpellDescription$ one",
		"SVar:DBB:DB$ GainLife | Defined$ You | LifeAmount$ 2 | SpellDescription$ two"), p, engine.Battlefield)
	c := engine.NewScriptedController()
	advanceToPhase(t, g, c, 1, engine.CombatBegin)
	activate := func() int {
		before := g.Player(p).Life
		c.QueueModeChoice([]int{0})
		if !g.ActivateAbility(p, charm, 0, c) {
			t.Fatal("ActivateAbility failed")
		}
		if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
			t.Fatal(err)
		}
		return g.Player(p).Life - before
	}
	if got := activate(); got != 1 {
		t.Fatalf("first combat gained %d, want 1", got)
	}
	if got := activate(); got != 1 {
		t.Fatalf("same combat, no restriction until it ends: gained %d, want 1", got)
	}
	advanceUntil(t, g, c, func() bool { return g.ActivePhase() == engine.Main2 })
	if got := activate(); got != 2 {
		t.Errorf("after combat gained %d, want 2: the first mode was chosen in the last combat", got)
	}
}
