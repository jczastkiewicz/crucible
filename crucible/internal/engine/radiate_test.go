package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
	"github.com/jczastkiewicz/crucible/internal/mana"
)

// Radiate: copy a spell that targets only one permanent or player once for each
// other permanent or player it could target, each copy targeting a different one.
func TestRadiateCopiesAShockForEveryOtherLegalTarget(t *testing.T) {
	t.Parallel()

	g, p, other := newTwoPlayerGameOn(t, scenarioDB(t))
	g.SetTurnState(2, other, engine.Main1)
	mine := g.NewCard(creatureDefPT(t, "2", "9"), p, engine.Battlefield)
	theirs := g.NewCard(creatureDefPT(t, "2", "9"), other, engine.Battlefield)
	shock := g.NewCard(corpusCard(t, "Shock"), other, engine.Hand)
	radiate := g.NewCard(corpusCard(t, "Radiate"), p, engine.Hand)
	g.Player(other).ManaPool.Add(mana.Red, 1)
	g.Player(p).ManaPool.Add(mana.Red, 5)
	oc := engine.NewScriptedController()
	oc.QueueTargets([]engine.EntityID{engine.CardEntity(mine)})
	if !g.CastSpell(other, shock, oc) {
		t.Fatalf("Shock: %v", g.TakePendingError())
	}
	c := engine.NewScriptedController()
	c.QueueTargets([]engine.EntityID{engine.CardEntity(shock)})
	for range 3 {
		c.QueuePayGeneric(mana.ShardR)
	}
	if !g.CastSpell(p, radiate, c) {
		t.Fatalf("Radiate: %v", g.TakePendingError())
	}
	if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
		t.Fatalf("ResolveStack: %v (pending %v)", err, g.TakePendingError())
	}
	if got := g.Card(mine).Damage.Marked; got != 2 {
		t.Errorf("my creature has %d damage, want 2 (the original)", got)
	}
	if got := g.Card(theirs).Damage.Marked; got != 2 {
		t.Errorf("their creature has %d damage, want 2 (one copy)", got)
	}
	if g.Player(p).Life != 18 || g.Player(other).Life != 18 {
		t.Errorf("life = %d and %d, want 18 and 18 (a copy each)", g.Player(p).Life, g.Player(other).Life)
	}
}
