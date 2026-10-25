package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
	"github.com/jczastkiewicz/crucible/internal/mana"
)

// Storm the Citadel: the granted DamageDone trigger names the defending
// player the damage source faced when it dealt the damage (Card.java:6343), so
// "target artifact or enchantment defending player controls" reaches only the
// defender's permanents.
func TestStormTheCitadelDestroysTheDefendersArtifact(t *testing.T) {
	t.Parallel()

	g, p, other := newTwoPlayerGameOn(t, scenarioDB(t))
	c := engine.NewScriptedController()
	g.SetTurnState(1, p, engine.Main1)
	attacker := g.NewCard(corpusCard(t, "Grizzly Bears"), p, engine.Battlefield)
	g.Card(attacker).SummonSick = false
	theirs := g.NewCard(corpusCard(t, "Sol Ring"), other, engine.Battlefield)
	storm := g.NewCard(corpusCard(t, "Storm the Citadel"), p, engine.Hand)
	g.Player(p).ManaPool.Add(mana.Green, 5)
	for i := 0; i < 4; i++ {
		c.QueuePayGeneric(mana.ShardG)
	}
	if !g.CastSpell(p, storm, c) {
		t.Fatalf("CastSpell failed: %v", g.TakePendingError())
	}
	if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}

	ac := engine.NewScriptedController()
	ac.QueueAttackers([]engine.CardID{attacker})
	declareAttackers(t, g, ac)
	bc := engine.NewScriptedController()
	bc.QueueBlocks(nil)
	declareBlockers(t, g, bc)
	c.QueueTargets([]engine.EntityID{engine.CardEntity(theirs)})
	g.DealCombatDamage(c)
	if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
		t.Fatalf("ResolveStack after damage: %v (pending %v)", err, g.TakePendingError())
	}
	if z := g.Card(theirs).Zone; z != engine.Graveyard {
		t.Errorf("the defender's artifact is in %v, want Graveyard", z)
	}
}
