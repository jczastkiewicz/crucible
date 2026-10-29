package engine_test

import (
	"slices"
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
	"github.com/jczastkiewicz/crucible/internal/mana"
)

// Mode$ IgnoreHexproof | Activator$ You | ValidEntity$ Creature.OppCtrl
// (Glaring Spotlight): its controller may target an opponent's hexproof
// creature (StaticAbilityIgnoreHexproofShroud.java:13-43).
func TestIgnoreHexproofLetsYouTargetAHexproofCreature(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		static string
		want   bool
	}{{"", false}, {"Glaring Spotlight", true}} {
		t.Run("with "+tc.static, func(t *testing.T) {
			t.Parallel()
			g, p, other := newTwoPlayerGameOn(t, scenarioDB(t))
			g.SetTurnState(1, p, engine.Main1)
			if tc.static != "" {
				g.NewCard(corpusCard(t, tc.static), p, engine.Battlefield)
			}
			rhino := g.NewCard(corpusCard(t, "Rubbleback Rhino"), other, engine.Battlefield)
			shock := g.NewCard(corpusCard(t, "Shock"), p, engine.Hand)
			g.Player(p).ManaPool.Add(mana.Red, 1)
			c := &targetRecorder{ScriptedController: engine.NewScriptedController()}
			c.QueueTargets([]engine.EntityID{engine.PlayerEntity(other)})

			if !g.CastSpell(p, shock, c) {
				t.Fatalf("cast failed: %v", g.TakePendingError())
			}
			offered := len(c.offers) == 1 && slices.Contains(c.offers[0], engine.CardEntity(rhino))
			if offered != tc.want {
				t.Errorf("the hexproof Rhino offered as a target = %v, want %v", offered, tc.want)
			}
		})
	}
}

// targetRecorder records every ChooseTargets candidate list.
type targetRecorder struct {
	*engine.ScriptedController
	offers [][]engine.EntityID
}

func (r *targetRecorder) ChooseTargets(g *engine.Game, p engine.PlayerID, candidates []engine.EntityID, lo, hi int) []engine.EntityID {
	r.offers = append(r.offers, append([]engine.EntityID(nil), candidates...))
	return r.ScriptedController.ChooseTargets(g, p, candidates, lo, hi)
}
