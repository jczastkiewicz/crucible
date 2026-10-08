package engine_test

import (
	"slices"
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
	"github.com/jczastkiewicz/crucible/internal/mana"
)

// TestGrantedKeywordNamesTheCardsManaCost proves CardManaCost in an AddKeyword$
// token becomes the affected card's ManaCost.getShortString (StaticAbilityContinuous
// .java:735-741): "Scavenge:CardManaCost" on a {2}{G}{G} card is "Scavenge:2 {G} {G}".
func TestGrantedKeywordNamesTheCardsManaCost(t *testing.T) {
	t.Parallel()

	g, p := textGame(t)
	g.NewCard(scriptDef(t, "Scavenger", "Enchantment",
		"S:Mode$ Continuous | Affected$ Creature.YouCtrl | AddKeyword$ Scavenge:CardManaCost"), p, engine.Battlefield)
	def := creatureDefPT(t, "2", "2")
	def.Faces[0].ManaCost = mana.MustParse("2 G G")
	bears := g.NewCard(def, p, engine.Battlefield)
	free := creatureDefPT(t, "1", "1")
	free.Faces[0].ManaCost = mana.GenericCost(0)
	token := g.NewCard(free, p, engine.Battlefield)
	sba(g)

	if !slices.Contains(g.Card(bears).KeywordLines(), "Scavenge:2 {G} {G}") {
		t.Errorf("bears keyword lines = %v, want Scavenge:2 {G} {G}", g.Card(bears).KeywordLines())
	}
	if !slices.Contains(g.Card(token).KeywordLines(), "Scavenge:0") {
		t.Errorf("zero-cost keyword lines = %v, want Scavenge:0", g.Card(token).KeywordLines())
	}
}

// TestGrantedAbilityBodyNamesTheCardsCost proves the same substitution in a
// granted activated ability (StaticAbilityContinuous.java:777-784): its Cost$
// reads the receiving card's short mana cost, and ConvertedManaCost its mana
// value when no CardManaCost is present.
func TestGrantedAbilityBodyNamesTheCardsCost(t *testing.T) {
	t.Parallel()

	g, p := textGame(t)
	g.NewCard(scriptDef(t, "Granter", "Enchantment",
		"S:Mode$ Continuous | Affected$ Creature.YouCtrl | AddAbility$ ABPay | AddStaticAbility$ STCmc",
		"SVar:ABPay:AB$ Pump | Cost$ CardManaCost | Defined$ Self | NumAtt$ 1",
		"SVar:STCmc:Mode$ Continuous | Affected$ Card.Self | AddPower$ ConvertedManaCost"), p, engine.Battlefield)
	def := creatureDefPT(t, "2", "2")
	def.Faces[0].ManaCost = mana.MustParse("2 G")
	bears := g.NewCard(def, p, engine.Battlefield)
	sba(g)

	wantPT(t, g, bears, 5, 2) // the granted static adds ConvertedManaCost = 3
	g.Player(p).ManaPool.Add(mana.Green, 3)
	c := engine.NewScriptedController()
	c.QueuePayGeneric(mana.ShardG)
	c.QueuePayGeneric(mana.ShardG)
	if !g.ActivateAbility(p, bears, 0, c) {
		t.Fatal("ActivateAbility(granted, Cost$ CardManaCost) = false")
	}
	if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
		t.Fatal(err)
	}
	sba(g)
	wantPT(t, g, bears, 6, 2)
	if left := g.Player(p).ManaPool.Total(); left != 0 {
		t.Errorf("%d mana left, want the whole 2 {G} cost paid", left)
	}
}

// TestAddAllCreatureTypesMakesEveryType proves AddAllCreatureTypes$ (Amorphous
// Axe: "equipped creature is every creature type"): the equipped creature
// counts as any creature type while the line applies, and loses it with it.
func TestAddAllCreatureTypesMakesEveryType(t *testing.T) {
	t.Parallel()

	g, p := textGame(t)
	axe := g.NewCard(corpusCard(t, "Amorphous Axe"), p, engine.Battlefield)
	bears := g.NewCard(corpusCard(t, "Grizzly Bears"), p, engine.Battlefield)
	g.Attach(axe, bears)
	sba(g)
	ty := g.Card(bears).Type()
	if !ty.HasSubtype("Goblin") || !ty.HasSubtype("Bear") || !ty.HasSubtype("Wizard") {
		t.Errorf("equipped bears type = %v, want every creature type", ty)
	}
	g.Unattach(axe)
	sba(g)
	if ty := g.Card(bears).Type(); ty.HasSubtype("Goblin") {
		t.Errorf("unequipped bears type = %v, want only Bear", ty)
	}
}

// TestAddAllCreatureTypesNeedsTheVocabulary proves a DB without a type list
// does nothing rather than half of the line (GO-7).
func TestAddAllCreatureTypesNeedsTheVocabulary(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGame(t)
	host := g.NewCard(scriptDef(t, "Shifter", "Creature",
		"S:Mode$ Continuous | Affected$ Card.Self | CharacteristicDefining$ True | AddAllCreatureTypes$ True"), p, engine.Battlefield)
	sba(g)
	if len(g.Card(host).Type().Subtypes()) != 0 {
		t.Errorf("subtypes = %v, want none without a vocabulary", g.Card(host).Type().Subtypes())
	}
}
