package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/cardtype"
	"github.com/jczastkiewicz/crucible/internal/engine"
)

// CR 613.8a: Kormus Bell ("All Swamps are 1/1 creatures") applies to what
// Urborg ("Each land is a Swamp") makes a Swamp, so it depends on Urborg and
// applies after it even with the earlier timestamp. A Forest therefore
// becomes a creature (GameAction.findStaticAbilityToApply).
func TestDependentLayerFourEffectAppliesAfterItsDependency(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGameOn(t, scenarioDB(t))
	g.SetTurnState(1, p, engine.Main1)
	g.NewCard(corpusCard(t, "Kormus Bell"), p, engine.Battlefield)
	g.NewCard(corpusCard(t, "Urborg, Tomb of Yawgmoth"), p, engine.Battlefield)
	forest := g.NewCard(corpusCard(t, "Forest"), p, engine.Battlefield)
	engine.CheckStateBasedActions(g, engine.NewScriptedController())

	ty := g.Card(forest).Type()
	if !ty.HasSubtype("Swamp") {
		t.Fatal("Urborg did not make the Forest a Swamp")
	}
	if !ty.Has(cardtype.Creature) {
		t.Error("Forest is not a creature: Kormus Bell applied before the Urborg effect it depends on")
	}
}

// CR 613.8b: a dependency loop is ignored, but only the loop. X ("Forests
// are Islands too") and Y ("Islands lose Forest") change what each other
// applies to through Breeding Pool, a Forest Island; A ("Islands are
// creatures", earliest) depends on X, which makes more Islands. Breaking the
// X-Y loop leaves Y first (earlier than X), then X, then A: the Forest, an
// Island by then, becomes a creature. Treating A as free because every
// static has an edge (no loop breaking), or plain timestamp order, applies A
// first and misses it.
func TestDependencyLoopIsIgnoredButNotTheEffectsDependingOnIt(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGameOn(t, scenarioDB(t))
	g.SetTurnState(1, p, engine.Main1)
	g.NewCard(scriptDef(t, "Test A", "Enchantment", "S:Mode$ Continuous | Affected$ Island | AddType$ Creature"), p, engine.Battlefield)
	g.NewCard(scriptDef(t, "Test Y", "Enchantment", "S:Mode$ Continuous | Affected$ Island | RemoveType$ Forest"), p, engine.Battlefield)
	g.NewCard(scriptDef(t, "Test X", "Enchantment", "S:Mode$ Continuous | Affected$ Forest | AddType$ Island"), p, engine.Battlefield)
	pool := g.NewCard(corpusCard(t, "Breeding Pool"), p, engine.Battlefield)
	forest := g.NewCard(corpusCard(t, "Forest"), p, engine.Battlefield)
	engine.CheckStateBasedActions(g, engine.NewScriptedController())

	if ty := g.Card(forest).Type(); !ty.Has(cardtype.Creature) || !ty.HasSubtype("Island") || !ty.HasSubtype("Forest") {
		t.Errorf("Forest types = %v, want a Forest Island creature (Y, X, then A)", ty)
	}
	if ty := g.Card(pool).Type(); !ty.Has(cardtype.Creature) || ty.HasSubtype("Forest") {
		t.Errorf("Breeding Pool types = %v, want an Island creature that lost Forest", ty)
	}
}
