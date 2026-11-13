package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
	"github.com/jczastkiewicz/crucible/internal/mana"
)

// A mana ability is restricted as any activated ability is
// (SpellAbilityRestriction.canPlay): Mox Opal's Activation$ Metalcraft and
// Riverpyre Verge's IsPresent$ decide whether it can be activated, never
// refuse it outright.
func TestManaAbilityReadsItsActivationRestriction(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name, card string
		index      int
		others     []string
		color      mana.Colors
		want       bool
	}{
		{"metalcraft met", "Mox Opal", 0, []string{"Ornithopter", "Ornithopter"}, mana.Red, true},
		{"metalcraft not met", "Mox Opal", 0, []string{"Ornithopter"}, mana.Red, false},
		{"island present", "Riverpyre Verge", 1, []string{"Island"}, 0, true},
		{"nothing present", "Riverpyre Verge", 1, []string{"Forest"}, 0, false},
	} {
		g, p, _ := newTwoPlayerGameOn(t, scenarioDB(t))
		g.SetTurnState(1, p, engine.Main1)
		src := g.NewCard(corpusCard(t, tc.card), p, engine.Battlefield)
		g.Card(src).SummonSick = false
		for _, o := range tc.others {
			g.NewCard(corpusCard(t, o), p, engine.Battlefield)
		}
		c := engine.NewScriptedController()
		if tc.color != 0 {
			c.QueueManaColor(tc.color)
		}
		if got := g.ActivateManaAbility(p, src, tc.index, c); got != tc.want {
			t.Errorf("%s: activated %v, want %v", tc.name, got, tc.want)
		}
	}
}
