package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/cardtype"
	"github.com/jczastkiewicz/crucible/internal/engine"
)

// CR 613.1 / ADR-0025: Layer 7 sees this pass's Layer 4. Kormus Bell makes a
// Swamp a 1/1 creature in Layer 4; Glorious Anthem's "creatures you control"
// (Layer 7c) must count it in the same pass, so one check gives a 2/2.
func TestLayerSevenSeesThisPassLayerFourTypes(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGameOn(t, scenarioDB(t))
	g.SetTurnState(1, p, engine.Main1)
	swamp := g.NewCard(corpusCard(t, "Swamp"), p, engine.Battlefield)
	g.NewCard(corpusCard(t, "Kormus Bell"), p, engine.Battlefield)
	g.NewCard(corpusCard(t, "Glorious Anthem"), p, engine.Battlefield)
	engine.CheckStateBasedActions(g, engine.NewScriptedController())

	c := g.Card(swamp)
	if !c.Type().Has(cardtype.Creature) {
		t.Fatal("Kormus Bell did not make the Swamp a creature")
	}
	if got, _ := c.Power(); got != 2 {
		t.Errorf("Swamp power after one pass = %d, want 2 (1/1 from Kormus Bell, +1/+1 from the anthem)", got)
	}
}

// CR 613.7: within a layer, statics are evaluated in timestamp order, not in
// battlefield-walk order. Urborg (earlier) makes every land a Swamp before
// Kormus Bell (later) picks its Swamps, so the Forest becomes a creature even
// though Kormus Bell's controller is walked first.
func TestLayerStaticsEvaluateInTimestampOrder(t *testing.T) {
	t.Parallel()

	g, p, other := newTwoPlayerGameOn(t, scenarioDB(t))
	g.SetTurnState(1, p, engine.Main1)
	g.NewCard(corpusCard(t, "Urborg, Tomb of Yawgmoth"), other, engine.Battlefield)
	g.NewCard(corpusCard(t, "Kormus Bell"), p, engine.Battlefield)
	forest := g.NewCard(corpusCard(t, "Forest"), p, engine.Battlefield)
	engine.CheckStateBasedActions(g, engine.NewScriptedController())

	if !g.Card(forest).Type().Has(cardtype.Creature) {
		t.Error("Forest is not a creature: Kormus Bell was evaluated before the earlier Urborg made it a Swamp")
	}
}
