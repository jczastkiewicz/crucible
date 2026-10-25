package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
	"github.com/jczastkiewicz/crucible/internal/mana"
)

// Favored Enemy notes the most prevalent creature type in an opponent's
// library as it enters; a library with no creature has none, which Java treats
// as nothing to note (ChooseTypeEffect.java:122-123), not as an error.
func TestFavoredEnemyEntersWithNoCreatureInTheLibrary(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGameOn(t, scenarioDB(t))
	c := engine.NewScriptedController()
	g.SetTurnState(1, p, engine.Main1)
	mine := g.NewCard(corpusCard(t, "Grizzly Bears"), p, engine.Battlefield)
	enemy := g.NewCard(corpusCard(t, "Favored Enemy"), p, engine.Hand)
	g.Player(p).ManaPool.Add(mana.Green, 4)
	for range 3 {
		c.QueuePayGeneric(mana.ShardG)
	}
	c.QueuePlayerChoice(g.Players()[1])
	c.QueueTargets([]engine.EntityID{engine.CardEntity(mine)})
	if !g.CastSpell(p, enemy, c) {
		t.Fatalf("CastSpell failed: %v", g.TakePendingError())
	}
	if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
		t.Fatalf("ResolveStack: %v (pending %v)", err, g.TakePendingError())
	}
	if z := g.Card(enemy).Zone; z != engine.Battlefield {
		t.Errorf("Favored Enemy is in %v, want Battlefield", z)
	}
}
