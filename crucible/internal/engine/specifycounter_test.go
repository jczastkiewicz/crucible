package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
	"github.com/jczastkiewicz/crucible/internal/mana"
)

// Exotic Pets puts one counter of each kind among your creatures on either
// Fish (SpecifyCounter$ with Choices$ Creature.IsRemembered): the chosen
// token gets it, the spell none (a Java test on upstream/master agrees).
func TestExoticPetsPutsEachKindOnAChosenFish(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGameOn(t, scenarioDB(t))
	g.SetTurnState(1, p, engine.Main1)
	bears := g.NewCard(corpusCard(t, "Grizzly Bears"), p, engine.Battlefield)
	g.Card(bears).Counters.Add(engine.P1P1, 1)
	pets := g.NewCard(corpusCard(t, "Exotic Pets"), p, engine.Hand)
	sba(g)
	g.Player(p).ManaPool.Add(mana.White, 1)
	g.Player(p).ManaPool.Add(mana.Blue, 2)
	c := engine.NewScriptedController()
	c.QueuePayGeneric(mana.ShardU)
	if !g.CastSpell(p, pets, c) {
		t.Fatalf("Exotic Pets not cast: %v", g.TakePendingError())
	}
	c.QueueCardChoice([]engine.CardID{pets + 1}) // the first Fish token
	if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
		t.Fatal(err)
	}
	onFish := 0
	for _, id := range g.Zone(engine.Battlefield, p).Cards() {
		if id != bears {
			onFish += g.Card(id).Counters.Count(engine.P1P1)
		}
	}
	if onFish != 1 || g.Card(pets).Counters.Count(engine.P1P1) != 0 {
		t.Errorf("%d +1/+1 counters on the Fish, %d on the spell; want 1 and 0", onFish, g.Card(pets).Counters.Count(engine.P1P1))
	}
}
