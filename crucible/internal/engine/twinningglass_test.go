package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
	"github.com/jczastkiewicz/crucible/internal/mana"
)

// Twinning Glass: "you may cast a spell from your hand without paying its mana
// cost if it has the same name as a spell that was cast this turn" -- only the
// copy of the cast spell qualifies, not the other card in hand.
func TestTwinningGlassCastsOnlyACardSharingANameWithATurnsSpell(t *testing.T) {
	t.Parallel()

	g, p, other := newTwoPlayerGameOn(t, scenarioDB(t))
	g.SetTurnState(1, p, engine.Main1)
	c := engine.NewScriptedController()
	glass := g.NewCard(corpusCard(t, "Twinning Glass"), p, engine.Battlefield)
	first := g.NewCard(corpusCard(t, "Lightning Bolt"), p, engine.Hand)
	twin := g.NewCard(corpusCard(t, "Lightning Bolt"), p, engine.Hand)
	g.NewCard(corpusCard(t, "Shock"), p, engine.Hand)
	g.Player(p).ManaPool.Add(mana.Red, 3)
	c.QueueTargets([]engine.EntityID{engine.PlayerEntity(other)})
	if !g.CastSpell(p, first, c) {
		t.Fatalf("CastSpell failed: %v", g.TakePendingError())
	}
	if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	c.QueueConfirmEffect(true)
	c.QueueTargets([]engine.EntityID{engine.PlayerEntity(other)})
	c.QueuePayGeneric(mana.ShardR)
	if !g.ActivateAbility(p, glass, 0, c) {
		t.Fatalf("ActivateAbility failed: %v", g.TakePendingError())
	}
	if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
		t.Fatalf("ResolveStack: %v (pending %v)", err, g.TakePendingError())
	}
	if z := g.Card(twin).Zone; z != engine.Graveyard {
		t.Errorf("the second Bolt is in %v, want Graveyard (cast free)", z)
	}
	if got := g.Player(other).Life; got != 14 {
		t.Errorf("opponent life = %d, want 14 (two Bolts)", got)
	}
}
