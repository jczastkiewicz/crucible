package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
	"github.com/jczastkiewicz/crucible/internal/mana"
)

// A Charm whose mode count, bounds or pawprints cannot be satisfied is not
// cast (GO-7: the script or the chooser is wrong, the game goes on), and a
// CharmNum$ above the number of modes is cut to the modes there are.
func TestCharmModeCountAndBounds(t *testing.T) {
	t.Parallel()

	const modeA = "SVar:DBA:DB$ GainLife | Defined$ You | LifeAmount$ 1 | Pawprint$ 2"
	const modeB = "SVar:DBB:DB$ GainLife | Defined$ You | LifeAmount$ 2 | Pawprint$ 2"
	for _, tc := range []struct {
		name   string
		charm  string
		chosen []int
		want   bool
		life   int
	}{
		{"unresolvable CharmNum$", "CharmNum$ Bogus", []int{0}, false, 20},
		{"unresolvable MinCharmNum$", "CharmNum$ 1 | MinCharmNum$ Bogus", []int{0}, false, 20},
		{"minimum above the modes", "CharmNum$ 5 | MinCharmNum$ 3", []int{0}, false, 20},
		{"CharmNum$ cut to the modes", "CharmNum$ 5 | MinCharmNum$ 1", []int{0, 1}, true, 23},
		{"too many modes chosen", "CharmNum$ 1 | MinCharmNum$ 1", []int{0, 1}, false, 20},
		{"random modes are not resolvable", "CharmNum$ 1 | Random$ False", []int{0}, false, 20},
		{"pawprints over the budget", "CharmNum$ 3 | Pawprint$ True", []int{0, 1}, false, 20},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			g, p, _ := newTwoPlayerGameOn(t, scenarioDB(t))
			g.Player(p).Life = 20
			spell := g.NewCard(copyTestDef(t, "Test Charm Shape", "Instant", "", "",
				"Cost:G", "A:SP$ Charm | Choices$ DBA,DBB | "+tc.charm, modeA, modeB), p, engine.Hand)
			g.Player(p).ManaPool.Add(mana.Green, 1)
			c := engine.NewScriptedController()
			c.QueueModeChoice(tc.chosen)
			// The error shows either as a refused cast or as the resolution's
			// error, depending on where the mode choice is checked.
			ok := g.CastSpell(p, spell, c)
			var err error
			if ok {
				err = g.ResolveStack(engine.NewRegistry(), c)
			}
			if failed := !ok || err != nil; failed == tc.want {
				t.Fatalf("cast %v, resolve error %v; want success %v", ok, err, tc.want)
			}
			if got := g.Player(p).Life; tc.want && got != tc.life {
				t.Errorf("life = %d, want %d", got, tc.life)
			}
		})
	}
}
