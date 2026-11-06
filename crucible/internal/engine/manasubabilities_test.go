package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
	"github.com/jczastkiewicz/crucible/internal/mana"
)

// A mana ability's SubAbility$ resolves with it, at once (AbilityUtils.resolve's
// sub-abilities, CR 605.3a): Adarkar Wastes' "deals 1 damage to you".
func TestPainLandDealsDamageAsItMakesMana(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGameOn(t, scenarioDB(t))
	land := g.NewCard(corpusCard(t, "Adarkar Wastes"), p, engine.Battlefield)
	c := engine.NewScriptedController()
	c.QueueManaColor(mana.Blue)
	// Index 1: the {W}/{U} ability; index 0 adds {C}.
	if !g.ActivateManaAbility(p, land, 1, c) {
		t.Fatalf("ActivateManaAbility failed: %v", g.TakePendingError())
	}
	if err := g.TakePendingError(); err != nil {
		t.Fatalf("pending error: %v", err)
	}
	if got := g.Player(p).Life; got != 19 {
		t.Errorf("life = %d, want 19", got)
	}
	if got := g.Player(p).ManaPool.Total(); got != 1 {
		t.Errorf("mana in pool = %d, want 1", got)
	}
}

// A loyalty ability that adds mana is no mana ability (CR 605.1a,
// SpellAbility.java:401-403): Chandra, Dressed to Kill's +1 is refused as
// one, goes on the stack, and on resolution adds {R} and deals 1 damage to
// the target chosen as it was activated.
func TestLoyaltyManaAbilityUsesTheStack(t *testing.T) {
	t.Parallel()

	g, p, other := newTwoPlayerGameOn(t, scenarioDB(t))
	chandra := g.NewCard(corpusCard(t, "Chandra, Dressed to Kill"), p, engine.Battlefield)
	g.Card(chandra).Counters.Add(engine.Loyalty, 3)
	c := engine.NewScriptedController()
	if g.ActivateManaAbility(p, chandra, 0, c) {
		t.Fatal("ActivateManaAbility took a loyalty ability, want it refused")
	}
	c.QueueTargets([]engine.EntityID{engine.PlayerEntity(other)})
	if !g.ActivateAbility(p, chandra, 0, c) {
		t.Fatalf("ActivateAbility failed: %v", g.TakePendingError())
	}
	if got := g.Player(p).ManaPool.Total(); got != 0 {
		t.Errorf("mana in pool before resolution = %d, want 0", got)
	}
	if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if err := g.TakePendingError(); err != nil {
		t.Fatalf("pending error: %v", err)
	}
	if got := g.Player(other).Life; got != 19 {
		t.Errorf("opponent's life = %d, want 19", got)
	}
	if got := g.Player(p).ManaPool.Total(); got != 1 {
		t.Errorf("mana in pool = %d, want 1", got)
	}
}
