package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
	"github.com/jczastkiewicz/crucible/internal/mana"
)

// Explosive Singularity: "As an additional cost to cast this spell, you may
// tap any number of untapped creatures you control. This spell costs {1} less
// to cast for each creature tapped this way." Announce$ Tapped sets the
// count, the RaiseCost static adds tapXType<Tapped/Creature> and the
// ReduceCost static takes {1} off per creature (costmod.go raiseXFromSVar).
func TestExplosiveSingularityTapsCreaturesForLessMana(t *testing.T) {
	t.Parallel()

	g, p, other := newTwoPlayerGameOn(t, scenarioDB(t))
	g.SetTurnState(1, p, engine.Main1)
	spell := g.NewCard(corpusCard(t, "Explosive Singularity"), p, engine.Hand)
	var bears []engine.CardID
	for range 3 {
		bears = append(bears, g.NewCard(corpusCard(t, "Grizzly Bears"), p, engine.Battlefield))
	}
	g.Player(p).ManaPool.Add(mana.Red, 7)
	c := engine.NewScriptedController()
	c.QueueNumberChoice(3)
	c.QueueTapChoice(bears)
	for range 5 {
		c.QueuePayGeneric(mana.ShardR)
	}
	c.QueueTargets([]engine.EntityID{engine.PlayerEntity(other)})
	if !g.CastSpell(p, spell, c) {
		t.Fatalf("cast failed: %v", g.TakePendingError())
	}
	if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
		t.Fatal(err)
	}
	if got := g.Player(other).Life; got != 10 {
		t.Errorf("opponent life = %d, want 10", got)
	}
	for _, id := range bears {
		if !g.Card(id).Tapped {
			t.Errorf("%v untapped, want tapped for the cost", id)
		}
	}
	if left := g.Player(p).ManaPool.Total(); left != 0 {
		t.Errorf("mana left = %d, want 0 (8RR less 3)", left)
	}
}
