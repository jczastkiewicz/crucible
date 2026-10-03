package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/cardtype"
	"github.com/jczastkiewicz/crucible/internal/engine"
)

// RemoveAllAbilities$ (Humility, Dress Down): every creature loses the text its
// own card printed (CardTraitChanges' remove predicate) -- keywords, activated
// and mana abilities, triggers, replacement effects and static abilities.
func TestHumilityRemovesEveryKindOfPrintedAbility(t *testing.T) {
	t.Parallel()

	g, p, other := newTwoPlayerGameOn(t, scenarioDB(t))
	g.SetTurnState(1, p, engine.Main1)
	angel := g.NewCard(corpusCard(t, "Serra Angel"), p, engine.Battlefield)
	elves := g.NewCard(corpusCard(t, "Llanowar Elves"), p, engine.Battlefield)
	lord := g.NewCard(scriptDef(t, "Test Lord", "Creature Elf",
		"S:Mode$ Continuous | Affected$ Creature.Other+YouCtrl | AddKeyword$ Haste"), p, engine.Battlefield)
	g.Card(elves).SummonSick = false
	g.NewCard(corpusCard(t, "Humility"), other, engine.Battlefield)
	sba(g)

	if g.Card(angel).HasKeyword("Flying") || g.Card(angel).HasKeyword("Vigilance") {
		t.Error("Serra Angel kept a printed keyword under Humility")
	}
	if got, _ := g.Card(angel).Power(); got != 1 {
		t.Errorf("Serra Angel power = %d, want 1", got)
	}
	if g.ActivateManaAbility(p, elves, 0, engine.NewScriptedController()) {
		t.Error("Llanowar Elves tapped for mana under Humility")
	}
	if g.Card(angel).HasKeyword("Haste") {
		t.Error("the lord's static still granted haste: it lost its abilities")
	}
	_ = lord
}

// RemoveNonManaAbilities$ (Blood Sun) takes every ability but the mana ones:
// a land's printed mana ability stays, its other abilities go.
func TestRemoveNonManaAbilitiesKeepsManaAbilities(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGameOn(t, scenarioDB(t))
	g.SetTurnState(1, p, engine.Main1)
	g.NewCard(scriptDef(t, "Test Sun", "Enchantment", "S:Mode$ Continuous | Affected$ Land | RemoveNonManaAbilities$ True"), p, engine.Battlefield)
	vein := g.NewCard(corpusCard(t, "Crystal Vein"), p, engine.Battlefield)
	sba(g)
	c := engine.NewScriptedController()
	if !g.ActivateManaAbility(p, vein, 0, c) {
		t.Error("Crystal Vein's mana ability was removed by RemoveNonManaAbilities$")
	}
	if g.ActivateAbility(p, vein, 1, c) {
		t.Error("Crystal Vein's sacrifice ability survived RemoveNonManaAbilities$")
	}
}

// CR 613.6 and 613.8: an effect that has started to apply keeps applying even
// when its source loses its abilities. A static that makes Humility a creature
// is applied first (Layer 4), so Humility loses its own abilities in Layer 6 --
// and still sets the Angel to 1/1 in Layer 7, whichever entered first.
func TestAnEffectKeepsApplyingAfterItsSourceLosesItsAbilities(t *testing.T) {
	t.Parallel()

	for _, humilityFirst := range []bool{true, false} {
		g, p, _ := newTwoPlayerGameOn(t, scenarioDB(t))
		g.SetTurnState(1, p, engine.Main1)
		var humility engine.CardID
		maker := func() {
			g.NewCard(scriptDef(t, "Test Maker", "Enchantment", "S:Mode$ Continuous | Affected$ Enchantment.nonAura+Other | AddType$ Creature"), p, engine.Battlefield)
		}
		if humilityFirst {
			humility = g.NewCard(corpusCard(t, "Humility"), p, engine.Battlefield)
			maker()
		} else {
			maker()
			humility = g.NewCard(corpusCard(t, "Humility"), p, engine.Battlefield)
		}
		angel := g.NewCard(corpusCard(t, "Serra Angel"), p, engine.Battlefield)
		sba(g)

		if !g.Card(humility).Type().Has(cardtype.Creature) {
			t.Fatalf("humility first %v: Humility is not a creature", humilityFirst)
		}
		if got, _ := g.Card(angel).Power(); got != 1 || g.Card(angel).HasKeyword("Flying") {
			t.Errorf("humility first %v: Serra Angel power = %d, flying = %v, want 1 and none: Humility's effect keeps applying after it loses its own ability",
				humilityFirst, got, g.Card(angel).HasKeyword("Flying"))
		}
	}
}
