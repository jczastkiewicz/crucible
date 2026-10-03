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

// CR 613.7e: an Equipment gets a new timestamp each time it becomes attached,
// so of two power-setting Equipment the one attached last wins; re-attaching
// the first puts it ahead again.
func TestEquipmentIsRestampedWhenItBecomesAttached(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGameOn(t, scenarioDB(t))
	g.SetTurnState(1, p, engine.Main1)
	bears := g.NewCard(corpusCard(t, "Grizzly Bears"), p, engine.Battlefield)
	four := g.NewCard(scriptDef(t, "Four Blade", "Artifact Equipment", "S:Mode$ Continuous | AffectedDefined$ Equipped | SetPower$ 4"), p, engine.Battlefield)
	two := g.NewCard(scriptDef(t, "Two Blade", "Artifact Equipment", "S:Mode$ Continuous | AffectedDefined$ Equipped | SetPower$ 2"), p, engine.Battlefield)
	g.Attach(four, bears)
	g.Attach(two, bears)
	sba(g)
	if got, _ := g.Card(bears).Power(); got != 2 {
		t.Fatalf("power = %d, want 2: the second Equipment attached wins", got)
	}
	g.Attach(four, bears)
	sba(g)
	if got, _ := g.Card(bears).Power(); got != 4 {
		t.Errorf("power = %d, want 4: re-attaching Four Blade restamps it", got)
	}
}

// CR 613.6: an effect that starts to apply in one layer applies to the same
// objects in every other layer. The later "creatures with power 2 or less gain
// flying and have power 4" starts in Layer 6, when the Bears are 2/2; by Layer 7b
// the earlier "creatures have power 5" has already made them 5/2, and the effect
// still applies to them, ending on top at 4.
func TestAnEffectAppliesToTheSameObjectsInEveryLayer(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGameOn(t, scenarioDB(t))
	g.SetTurnState(1, p, engine.Main1)
	g.NewCard(scriptDef(t, "Test Five", "Enchantment", "S:Mode$ Continuous | Affected$ Creature | SetPower$ 5"), p, engine.Battlefield)
	g.NewCard(scriptDef(t, "Test Four", "Enchantment", "S:Mode$ Continuous | Affected$ Creature.powerLE2 | AddKeyword$ Flying | SetPower$ 4"), p, engine.Battlefield)
	bears := g.NewCard(corpusCard(t, "Grizzly Bears"), p, engine.Battlefield)
	sba(g)

	if !g.Card(bears).HasKeyword("Flying") {
		t.Error("the Bears did not gain flying in Layer 6")
	}
	if got, _ := g.Card(bears).Power(); got != 4 {
		t.Errorf("power = %d, want 4: the effect keeps applying to the Bears it started on", got)
	}
}
