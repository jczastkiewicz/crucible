package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
	"github.com/jczastkiewicz/crucible/internal/mana"
)

// Aang, Swift Savior: when he enters, airbend up to one other target creature
// or spell -- a spell on the stack is exiled and leaves the stack.
func TestAangAirbendsASpellOffTheStack(t *testing.T) {
	t.Parallel()

	g, p, other := newTwoPlayerGameOn(t, scenarioDB(t))
	g.SetTurnState(2, other, engine.Main1)
	mine := g.NewCard(creatureDefPT(t, "2", "9"), p, engine.Battlefield)
	shock := g.NewCard(corpusCard(t, "Shock"), other, engine.Hand)
	aang := g.NewCard(corpusCard(t, "Aang, Swift Savior"), p, engine.Hand)
	g.Player(other).ManaPool.Add(mana.Red, 1)
	g.Player(p).ManaPool.Add(mana.White, 2)
	g.Player(p).ManaPool.Add(mana.Blue, 1)
	oc := engine.NewScriptedController()
	oc.QueueTargets([]engine.EntityID{engine.CardEntity(mine)})
	if !g.CastSpell(other, shock, oc) {
		t.Fatalf("Shock: %v", g.TakePendingError())
	}
	c := engine.NewScriptedController()
	c.QueuePayGeneric(mana.ShardW)
	c.QueueTargets([]engine.EntityID{engine.CardEntity(shock)})
	if !g.CastSpell(p, aang, c) {
		t.Fatalf("Aang: %v", g.TakePendingError())
	}
	if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
		t.Fatalf("ResolveStack: %v (pending %v)", err, g.TakePendingError())
	}
	if z := g.Card(shock).Zone; z != engine.Exile {
		t.Errorf("the Shock is in %v, want Exile", z)
	}
	if got := g.Card(mine).Damage.Marked; got != 0 {
		t.Errorf("my creature has %d damage, want 0 (the Shock never resolved)", got)
	}
}
