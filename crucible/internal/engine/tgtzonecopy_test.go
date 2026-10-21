package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
)

// TgtZone$ Graveyard on Clone (Lazav, the Multifarious) and CopyPermanent
// (Dino DNA with Exile): the target is a card in that zone, copied by its
// printed card (CloneEffect.java:106, CopyPermanentEffect.java:290).
func TestCloneAndCopyPermanentTargetACardOffTheBattlefield(t *testing.T) {
	t.Parallel()

	t.Run("Clone", func(t *testing.T) {
		t.Parallel()
		g, p, _ := newTwoPlayerGameOn(t, scenarioDB(t))
		g.SetTurnState(1, p, engine.Main1)
		dead := g.NewCard(creatureDefPT(t, "5", "5"), p, engine.Graveyard)
		live := g.NewCard(creatureDefPT(t, "3", "3"), p, engine.Battlefield)
		cloner := g.NewCard(creatureDefWithAbility(t, "Test Cloner",
			"AB$ Clone | Cost$ 0 | ValidTgts$ Creature.YouOwn | TgtZone$ Graveyard"), p, engine.Battlefield)
		g.Card(cloner).SummonSick = false
		c := engine.NewScriptedController()
		c.QueueTargets([]engine.EntityID{engine.CardEntity(dead)})
		g.ActivateAbility(p, cloner, 0, c)
		if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
			t.Fatalf("ResolveStack: %v", err)
		}
		if pw, _ := g.Card(cloner).Power(); pw != 5 {
			t.Errorf("cloner power = %d, want 5 (the graveyard creature's)", pw)
		}
		if pw, _ := g.Card(live).Power(); pw != 3 {
			t.Errorf("battlefield creature power = %d, want 3 (not a legal target)", pw)
		}
	})

	t.Run("CopyPermanent", func(t *testing.T) {
		t.Parallel()
		g, p, _ := newTwoPlayerGameOn(t, scenarioDB(t))
		g.SetTurnState(1, p, engine.Main1)
		exiled := g.NewCard(creatureDefPT(t, "4", "4"), p, engine.Exile)
		maker := g.NewCard(creatureDefWithAbility(t, "Test Maker",
			"AB$ CopyPermanent | Cost$ 0 | Defined$ Targeted | ValidTgts$ Creature | TgtZone$ Exile"), p, engine.Battlefield)
		g.Card(maker).SummonSick = false
		before := len(g.Zone(engine.Battlefield, p).Cards())
		c := engine.NewScriptedController()
		c.QueueTargets([]engine.EntityID{engine.CardEntity(exiled)})
		g.ActivateAbility(p, maker, 0, c)
		if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
			t.Fatalf("ResolveStack: %v", err)
		}
		if got := len(g.Zone(engine.Battlefield, p).Cards()); got != before+1 {
			t.Errorf("battlefield = %d cards, want %d (one token copy)", got, before+1)
		}
	})
}
