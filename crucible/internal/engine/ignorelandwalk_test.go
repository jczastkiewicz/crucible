package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
)

// Mode$ IgnoreLandWalk | ValidKeyword$ Landwalk:Island (Undertow): an
// islandwalker attacking a player who controls an Island can be blocked as
// though it lacked islandwalk; without the static it cannot
// (StaticAbilityIgnoreLandwalk.java:26-38).
func TestIgnoreLandWalkLetsALandwalkerBeBlocked(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		static string
		want   bool
	}{{"", false}, {"Undertow", true}, {"Lord Magnus", false}} {
		t.Run("with "+tc.static, func(t *testing.T) {
			t.Parallel()
			g, a, d := newTwoPlayerGameOn(t, scenarioDB(t))
			g.SetTurnState(1, a, engine.DeclareBlockers)
			bear := g.NewCard(corpusCard(t, "River Bear"), a, engine.Battlefield)
			g.NewCard(corpusCard(t, "Island"), d, engine.Battlefield)
			blocker := g.NewCard(corpusCard(t, "Grizzly Bears"), d, engine.Battlefield)
			if tc.static != "" {
				g.NewCard(corpusCard(t, tc.static), d, engine.Battlefield)
			}
			if got := g.CanBlock(bear, blocker); got != tc.want {
				t.Errorf("CanBlock = %v, want %v", got, tc.want)
			}
		})
	}
}
