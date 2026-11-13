package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
	"github.com/jczastkiewicz/crucible/internal/mana"
)

// Devoid makes a card colorless in every zone (CR 702.114a): Basking
// Broodscale, cast from the hand, is a colorless Eldrazi spell that Eldrazi
// Temple's restricted {C}{C} may pay for.
func TestDevoidSpellIsColorlessForEldraziTempleMana(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGameOn(t, scenarioDB(t))
	g.SetTurnState(1, p, engine.Main1)
	temple := g.NewCard(corpusCard(t, "Eldrazi Temple"), p, engine.Battlefield)
	broodscale := g.NewCard(corpusCard(t, "Basking Broodscale"), p, engine.Hand)
	engine.CheckStateBasedActions(g, engine.NewScriptedController())
	if got := g.Card(broodscale).Colors(); got != 0 {
		t.Fatalf("Basking Broodscale's colors in hand = %v, want colorless", got)
	}
	c := engine.NewScriptedController()
	if !g.ActivateManaAbility(p, temple, 1, c) {
		t.Fatal("Eldrazi Temple's restricted mana ability not activated")
	}
	g.Player(p).ManaPool.Add(mana.Green, 1)
	c.QueuePayGeneric(mana.ShardC)
	if !g.CastSpell(p, broodscale, c) {
		t.Fatalf("Basking Broodscale not cast with the Temple's mana: %v", g.TakePendingError())
	}
	if got := g.Player(p).ManaPool.Breakdown(); got[5] != 1 {
		t.Errorf("pool %v, want one of the Temple's {C} left", got)
	}
}
