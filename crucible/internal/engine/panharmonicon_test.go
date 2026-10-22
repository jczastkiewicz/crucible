package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
)

// TestPanharmoniconRepeatsAnEntersTrigger proves StaticAbilityPanharmonicon
// (Panharmonicon): a creature's enters trigger fires an additional time per
// matching static, a static for another cause or another mode adds none, and
// an opponent's does not count for permanents you control.
func TestPanharmoniconRepeatsAnEntersTrigger(t *testing.T) {
	t.Parallel()

	const own = "S:Mode$ Panharmonicon | ValidMode$ ChangesZone | ValidCard$ Permanent.YouCtrl | Destination$ Battlefield | ValidCause$ Artifact,Creature"
	for _, tc := range []struct {
		name  string
		lines []string
		gain  int
	}{
		{"none", nil, 3},
		{"one", []string{own}, 6},
		{"two stack", []string{own, own}, 9},
		{"land cause only", []string{"S:Mode$ Panharmonicon | ValidMode$ ChangesZone | ValidCard$ Permanent.YouCtrl | Destination$ Battlefield | ValidCause$ Land"}, 3},
		{"another mode", []string{"S:Mode$ Panharmonicon | ValidMode$ Attacks | ValidCard$ Permanent.YouCtrl"}, 3},
		{"no ValidMode applies to every mode", []string{"S:Mode$ Panharmonicon | ValidCard$ Permanent.YouCtrl"}, 6},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			g, p, _ := newTwoPlayerGame(t)
			for _, line := range tc.lines {
				g.NewCard(scriptDef(t, "Doubler", "Artifact", line), p, engine.Battlefield)
			}
			before := g.Player(p).Life
			if _, err := castETBChain(t, g, p, etbChainDef(t, "Greeter", "DB$ GainLife | Defined$ You | LifeAmount$ 3"), engine.NewScriptedController()); err != nil {
				t.Fatalf("cast: %v", err)
			}
			if got := g.Player(p).Life - before; got != tc.gain {
				t.Errorf("gained %d, want %d", got, tc.gain)
			}
		})
	}
}
