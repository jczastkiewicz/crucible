package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
	"github.com/jczastkiewicz/crucible/internal/mana"
)

// Debt of Loyalty: its RegenerationAbility$ is a sub-ability of the shield's
// replacement, so it runs when the shield is spent with Defined$ ReplacedCard
// naming the regenerated creature, and the caster gains control of it then --
// not when the spell resolves.
func TestDebtOfLoyaltyGainsControlWhenTheShieldIsSpent(t *testing.T) {
	t.Parallel()

	g, p, other := newTwoPlayerGameOn(t, scenarioDB(t))
	c := engine.NewScriptedController()
	g.SetTurnState(1, p, engine.Main1)
	bears := g.NewCard(corpusCard(t, "Grizzly Bears"), other, engine.Battlefield)
	debt := g.NewCard(corpusCard(t, "Debt of Loyalty"), p, engine.Hand)
	g.Player(p).ManaPool.Add(mana.White, 3)
	c.QueuePayGeneric(mana.ShardW)
	c.QueueTargets([]engine.EntityID{engine.CardEntity(bears)})
	if !g.CastSpell(p, debt, c) {
		t.Fatalf("CastSpell failed: %v", g.TakePendingError())
	}
	if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if got := g.Card(bears).Controller(); got != other {
		t.Fatalf("controller before the shield is spent = %v, want the opponent", got)
	}
	c.QueueTargets([]engine.EntityID{engine.CardEntity(bears)})
	resolveLine(t, g, p, c, "DB$ Destroy | ValidTgts$ Creature")
	if z := g.Card(bears).Zone; z != engine.Battlefield {
		t.Fatalf("the bears are in %v, want Battlefield (regenerated)", z)
	}
	if got := g.Card(bears).Controller(); got != p {
		t.Errorf("controller after regenerating = %v, want the caster", got)
	}
}
