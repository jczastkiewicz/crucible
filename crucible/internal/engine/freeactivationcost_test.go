package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
	"github.com/jczastkiewicz/crucible/internal/mana"
)

// Bruenor Battlehammer (Activated.Equip, Cost$ 0, CheckSVar$ X < 1): the first
// equip activation of the turn may be paid with {0}; once one was activated the
// printed cost stands and the option is not offered.
func TestAlternativeCostMakesTheFirstEquipFree(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGameOn(t, scenarioDB(t))
	g.SetTurnState(1, p, engine.Main1)
	g.NewCard(corpusCard(t, "Bruenor Battlehammer"), p, engine.Battlefield)
	blade := g.NewCard(scriptDef(t, "Test Blade", "Artifact Equipment", "K:Equip:2"), p, engine.Battlefield)
	bears := g.NewCard(corpusCard(t, "Grizzly Bears"), p, engine.Battlefield)
	c := engine.NewScriptedController()
	c.QueueTargets([]engine.EntityID{engine.CardEntity(bears)})
	c.QueueOption(1)
	if !g.ActivateAbility(p, blade, 0, c) {
		t.Fatalf("free equip not activated: %v", g.TakePendingError())
	}
	if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
		t.Fatal(err)
	}
	other := g.NewCard(corpusCard(t, "Grizzly Bears"), p, engine.Battlefield)
	c.QueueTargets([]engine.EntityID{engine.CardEntity(other)})
	c.QueuePayGeneric(mana.ShardG)
	c.QueuePayGeneric(mana.ShardG)
	if g.ActivateAbility(p, blade, 0, c) {
		t.Error("second equip of the turn was free")
	}
	g.Player(p).ManaPool.Add(mana.Green, 2)
	c.QueuePayGeneric(mana.ShardG)
	c.QueuePayGeneric(mana.ShardG)
	c.QueueTargets([]engine.EntityID{engine.CardEntity(other)})
	if !g.ActivateAbility(p, blade, 0, c) {
		t.Errorf("paid second equip failed: %v", g.TakePendingError())
	}
}
