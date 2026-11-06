package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
	"github.com/jczastkiewicz/crucible/internal/mana"
)

// Entwine (CR 702.42): paying the entwine cost as the spell is cast chooses
// every mode (CharmEffect.java:228-232); declining asks for one as usual.
func TestEntwineChoosesEveryMode(t *testing.T) {
	t.Parallel()

	for _, entwine := range []bool{true, false} {
		g, p, other := newTwoPlayerGameOn(t, scenarioDB(t))
		g.SetTurnState(1, p, engine.Main1)
		untapped := g.NewCard(creatureDefPT(t, "2", "2"), other, engine.Battlefield)
		tapped := g.NewCard(creatureDefPT(t, "2", "2"), p, engine.Battlefield)
		g.Card(tapped).Tapped = true
		grip := g.NewCard(corpusCard(t, "Dream's Grip"), p, engine.Hand)
		g.Player(p).ManaPool.Add(mana.Blue, 2)

		c := engine.NewScriptedController()
		c.QueueConfirmPayCost(entwine)
		if entwine {
			c.QueueTargets([]engine.EntityID{engine.CardEntity(untapped)})
			c.QueueTargets([]engine.EntityID{engine.CardEntity(tapped)})
			c.QueuePayGeneric(mana.ShardU)
		} else {
			c.QueueModeChoice([]int{0})
			c.QueueTargets([]engine.EntityID{engine.CardEntity(untapped)})
		}
		castThenResolve(t, g, p, grip, c)
		if err := g.TakePendingError(); err != nil {
			t.Fatalf("entwine %v: %v", entwine, err)
		}
		if !g.Card(untapped).Tapped {
			t.Errorf("entwine %v: the tap mode did not resolve", entwine)
		}
		if got := g.Card(tapped).Tapped; got == entwine {
			t.Errorf("entwine %v: other permanent tapped = %v, want %v", entwine, got, !entwine)
		}
	}
}
