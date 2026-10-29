package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
)

// Mode$ BecomesPlotted fires each time Plotted is set (Card.setPlotted) and
// Mode$ CaseSolved only when Solved actually changes (AlterAttributeEffect
// java:91-97): a second Solve changes nothing, a second Plot fires again.
func TestAlterAttributeFiresPlottedAndSolvedTriggers(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name, mode, attr string
		want             []int // life gained after the first and the second alteration
	}{
		{"plotted", "BecomesPlotted", "Plotted", []int{1, 2}},
		{"solved", "CaseSolved", "Solved", []int{1, 1}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			g, p, _ := newTwoPlayerGame(t)
			watcher := g.NewCard(copyTestDef(t, "Watcher", "Creature Test", "1", "1",
				"T:Mode$ "+tc.mode+" | ValidCard$ Card.Self | TriggerZones$ Battlefield | Execute$ TrigGain",
				"SVar:TrigGain:DB$ GainLife | Defined$ You | LifeAmount$ 1"), p, engine.Battlefield)
			c := engine.NewScriptedController()
			start := g.Player(p).Life
			for i, want := range tc.want {
				line := "DB$ AlterAttribute | ValidTgts$ Creature | Attributes$ " + tc.attr + " | Activate$ True"
				if err := resolveTargeting(t, g, p, c, []engine.EntityID{engine.CardEntity(watcher)}, line); err != nil {
					t.Fatalf("alteration %d: %v", i+1, err)
				}
				if got := g.Player(p).Life - start; got != want {
					t.Errorf("after alteration %d: life gained %d, want %d", i+1, got, want)
				}
			}
		})
	}
}
