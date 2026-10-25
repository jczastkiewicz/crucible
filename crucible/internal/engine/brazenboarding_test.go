package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
	"github.com/jczastkiewicz/crucible/internal/mana"
)

// Brazen Boarding: the spellbook is filtered to the cards whose mana value
// equals the excess damage (MakeCardEffect.java:94-103) before the caster
// picks, so 2 excess damage offers only Staunch Crewmate.
func TestBrazenBoardingConjuresTheSpellbookCardMatchingTheExcess(t *testing.T) {
	t.Parallel()

	g, p, other := newTwoPlayerGameOn(t, scenarioDB(t))
	c := engine.NewScriptedController()
	g.SetTurnState(1, p, engine.Main1)
	bears := g.NewCard(corpusCard(t, "Grizzly Bears"), other, engine.Battlefield)
	boarding := g.NewCard(corpusCard(t, "Brazen Boarding"), p, engine.Hand)
	g.Player(p).ManaPool.Add(mana.Blue, 2)
	g.Player(p).ManaPool.Add(mana.Red, 2)
	c.QueuePayGeneric(mana.ShardU)
	c.QueuePayGeneric(mana.ShardR)
	c.QueueTargets([]engine.EntityID{engine.CardEntity(bears)})
	c.QueueOption(0)
	if !g.CastSpell(p, boarding, c) {
		t.Fatalf("CastSpell failed: %v", g.TakePendingError())
	}
	if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
		t.Fatalf("ResolveStack: %v (pending %v)", err, g.TakePendingError())
	}
	var found bool
	for _, id := range g.Zone(engine.Battlefield, p).Cards() {
		if g.Card(id).Def.Name == "Staunch Crewmate" {
			found = true
		}
	}
	if !found {
		t.Errorf("no Staunch Crewmate on the battlefield")
	}
}
