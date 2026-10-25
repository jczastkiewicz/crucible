package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
	"github.com/jczastkiewicz/crucible/internal/mana"
)

// Jeweled Amulet: {1},{T}: put a charge counter and note the type of mana spent
// (RememberCostMana$, one String on the host); {T}, remove the counter: add one
// mana of the noted type (Produced$ Special LastNotedType).
func TestJeweledAmuletAddsManaOfTheNotedType(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGameOn(t, scenarioDB(t))
	g.SetTurnState(1, p, engine.Main1)
	c := engine.NewScriptedController()
	amulet := g.NewCard(corpusCard(t, "Jeweled Amulet"), p, engine.Battlefield)
	g.Player(p).ManaPool.Add(mana.Green, 1)
	c.QueuePayGeneric(mana.ShardG)
	if !g.ActivateAbility(p, amulet, 0, c) {
		t.Fatalf("first activation failed: %v", g.TakePendingError())
	}
	if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
		t.Fatalf("ResolveStack: %v (pending %v)", err, g.TakePendingError())
	}
	g.Card(amulet).Tapped = false
	if !g.ActivateManaAbility(p, amulet, 1, c) {
		t.Fatalf("second activation failed: %v", g.TakePendingError())
	}
	if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
		t.Fatalf("ResolveStack: %v (pending %v)", err, g.TakePendingError())
	}
	if got := g.Player(p).ManaPool.Total(); got != 1 {
		t.Errorf("mana in pool = %d, want 1", got)
	}
}

// Ice Cauldron: {X},{T} exiles a nonland card from hand and notes the mana spent
// ("R" for X=1 paid with red); the mana it later adds is that noted mana,
// restricted to casting the exiled card (RestrictValid$ Spell.IsRemembered).
func TestIceCauldronNotesTheManaSpentAndAddsItRestricted(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGameOn(t, scenarioDB(t))
	g.SetTurnState(1, p, engine.Main1)
	c := engine.NewScriptedController()
	cauldron := g.NewCard(corpusCard(t, "Ice Cauldron"), p, engine.Battlefield)
	g.Card(cauldron).SummonSick = false
	bolt := g.NewCard(corpusCard(t, "Lightning Bolt"), p, engine.Hand)
	g.Player(p).ManaPool.Add(mana.Red, 1)
	c.QueuePayX(1)
	c.QueuePayGeneric(mana.ShardR)
	c.QueueConfirmEffect(true)
	c.QueueCardChoice([]engine.CardID{bolt})
	if !g.ActivateAbility(p, cauldron, 0, c) {
		t.Fatalf("first activation failed: %v", g.TakePendingError())
	}
	if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
		t.Fatalf("ResolveStack: %v (pending %v)", err, g.TakePendingError())
	}
	if z := g.Card(bolt).Zone; z != engine.Exile {
		t.Fatalf("the Bolt is in %v, want Exile", z)
	}
	g.Card(cauldron).Tapped = false
	if !g.ActivateManaAbility(p, cauldron, 1, c) {
		t.Fatalf("mana activation failed: %v", g.TakePendingError())
	}
	if got := g.Player(p).ManaPool.Total(); got != 1 {
		t.Errorf("mana in pool = %d, want the 1 noted red", got)
	}
}
