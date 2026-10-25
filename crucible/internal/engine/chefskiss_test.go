package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
	"github.com/jczastkiewicz/crucible/internal/mana"
)

// Chef's Kiss: gain control of a spell that targets a single permanent or
// player, copy it, then reselect the targets of the spell and the copy at
// random among players other than you and permanents you don't control.
func TestChefsKissRetargetsTheSpellAndItsCopyAwayFromYou(t *testing.T) {
	t.Parallel()

	g, p, other := newTwoPlayerGameOn(t, scenarioDB(t))
	g.SetTurnState(2, other, engine.Main1)
	mine := g.NewCard(creatureDefPT(t, "2", "9"), p, engine.Battlefield)
	theirs := g.NewCard(creatureDefPT(t, "2", "9"), other, engine.Battlefield)
	shock := g.NewCard(corpusCard(t, "Shock"), other, engine.Hand)
	kiss := g.NewCard(corpusCard(t, "Chef's Kiss"), p, engine.Hand)
	g.Player(other).ManaPool.Add(mana.Red, 1)
	g.Player(p).ManaPool.Add(mana.Red, 3)
	oc := engine.NewScriptedController()
	oc.QueueTargets([]engine.EntityID{engine.CardEntity(mine)})
	if !g.CastSpell(other, shock, oc) {
		t.Fatalf("Shock: %v", g.TakePendingError())
	}
	c := engine.NewScriptedController()
	c.QueueTargets([]engine.EntityID{engine.CardEntity(shock)})
	for range 2 {
		c.QueuePayGeneric(mana.ShardR)
	}
	if !g.CastSpell(p, kiss, c) {
		t.Fatalf("Chef's Kiss: %v", g.TakePendingError())
	}
	if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
		t.Fatalf("ResolveStack: %v (pending %v)", err, g.TakePendingError())
	}
	if got := g.Card(mine).Damage.Marked; got != 0 {
		t.Errorf("my creature has %d damage, want 0 (never a legal new target)", got)
	}
	dealt := g.Card(theirs).Damage.Marked + (20 - g.Player(other).Life)
	if dealt != 4 {
		t.Errorf("damage on the opponent's side = %d, want 4 (two Shocks)", dealt)
	}
	if got := g.Player(p).Life; got != 20 {
		t.Errorf("my life = %d, want 20", got)
	}
}
