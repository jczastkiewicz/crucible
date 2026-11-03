package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
	"github.com/jczastkiewicz/crucible/internal/mana"
)

// A pumped keyword's CardManaCost names the pumped card's own mana cost
// (PumpEffect.java:453-466): Confession Dial's grant gives a graveyard Grizzly
// Bears escape {1}{G} plus three other graveyard cards, and it escapes.
func TestPumpedKeywordCostIsTheTargetsManaCost(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGameOn(t, scenarioDB(t))
	bears := g.NewCard(corpusCard(t, "Grizzly Bears"), p, engine.Graveyard)
	fodder := addN(t, g, p, engine.Graveyard, 3, "Artifact")
	if _, err := resolveNow(t, g, p, engine.NewScriptedController(), []engine.EntityID{engine.CardEntity(bears)},
		"DB$ Pump | TgtZone$ Graveyard | ValidTgts$ Creature.YouOwn | PumpZone$ Graveyard | KW$ Escape:CardManaCost ExileFromGrave<3/Card.Other/other>"); err != nil {
		t.Fatal(err)
	}
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).ManaPool.Add(mana.Green, 2)
	c := engine.NewScriptedController()
	c.QueuePayGeneric(mana.ShardG)
	c.QueueCardChoice(fodder)
	castThenResolve(t, g, p, bears, c)
	if got := g.Card(bears).Zone; got != engine.Battlefield {
		t.Fatalf("Grizzly Bears in %v, want it escaped onto the battlefield", got)
	}
	if left := g.Player(p).ManaPool.Total(); left != 0 {
		t.Errorf("%d mana left in the pool, want the escape cost {1}{G} paid in full", left)
	}
}
