package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
	"github.com/jczastkiewicz/crucible/internal/mana"
)

// A hidden-zone ChangeZone whose DefinedPlayer$ names every player resolves
// the players' choices in APNAP order (ChangeZoneEffect.java:910-911,
// sortAPNAP): Show and Tell cast on the second seat's turn asks that player
// first, then the first seat.
func TestEachPlayerSearchAsksTheActivePlayerFirst(t *testing.T) {
	t.Parallel()

	g, p, other := newTwoPlayerGameOn(t, scenarioDB(t))
	g.SetTurnState(2, other, engine.Main1)
	giant := g.NewCard(corpusCard(t, "Hill Giant"), p, engine.Hand)
	bears := g.NewCard(corpusCard(t, "Grizzly Bears"), other, engine.Hand)
	show := g.NewCard(corpusCard(t, "Show and Tell"), other, engine.Hand)
	g.Player(other).ManaPool.Add(mana.Blue, 3)
	c := engine.NewScriptedController()
	c.QueuePayGeneric(mana.ShardU)
	c.QueuePayGeneric(mana.ShardU)
	c.QueueCardChoice([]engine.CardID{bears})
	c.QueueCardChoice([]engine.CardID{giant})
	if !g.CastSpell(other, show, c) {
		t.Fatalf("CastSpell failed: %v", g.TakePendingError())
	}
	if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	for _, id := range []engine.CardID{giant, bears} {
		if z := g.Card(id).Zone; z != engine.Battlefield {
			t.Errorf("%s is in %v, want Battlefield", g.Card(id).Def.Name, z)
		}
	}
}
