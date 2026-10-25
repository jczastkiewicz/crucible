package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
	"github.com/jczastkiewicz/crucible/internal/mana"
)

// Close Encounter: choosing a creature is an additional cost, and the damage
// is that card's power (Revealed$CardPower). The warped-exile alternative never
// holds here, no card being warped.
func TestCloseEncounterDealsTheChosenCreaturesPower(t *testing.T) {
	t.Parallel()

	g, p, other := newTwoPlayerGameOn(t, scenarioDB(t))
	c := engine.NewScriptedController()
	g.SetTurnState(1, p, engine.Main1)
	mine := g.NewCard(creatureDefPT(t, "3", "3"), p, engine.Battlefield)
	theirs := g.NewCard(creatureDefPT(t, "2", "7"), other, engine.Battlefield)
	spell := g.NewCard(corpusCard(t, "Close Encounter"), p, engine.Hand)
	g.Player(p).ManaPool.Add(mana.Green, 2)
	c.QueuePayGeneric(mana.ShardG)
	c.QueueCardChoice([]engine.CardID{mine})
	c.QueueTargets([]engine.EntityID{engine.CardEntity(theirs)})
	if !g.CastSpell(p, spell, c) {
		t.Fatalf("CastSpell failed: %v", g.TakePendingError())
	}
	if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
		t.Fatalf("ResolveStack: %v (pending %v)", err, g.TakePendingError())
	}
	if got := g.Card(theirs).Damage.Marked; got != 3 {
		t.Errorf("damage = %d, want 3", got)
	}
}
