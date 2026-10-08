package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/cardtype"
	"github.com/jczastkiewicz/crucible/internal/engine"
)

// These tests cover Copy-layer replacements hosted by an effect card
// (Mystic Reflection) and chains that create one (Spark Double, Moritte of
// the Frost).

// mirrorLines is Mystic Reflection's effect as a permanent's activated
// ability: it targets a nonlegendary creature and leaves an effect card
// whose Copy-layer replacement makes the next creatures to enter copies of it.
var mirrorLines = []string{
	"A:AB$ Effect | Cost$ 0 | ValidTgts$ Creature.nonLegendary | TgtPrompt$ Choose target nonlegendary creature | RememberObjects$ Targeted | ReplacementEffects$ ReplaceETB | Triggers$ TrigRemove",
	"SVar:ReplaceETB:Event$ Moved | Destination$ Battlefield | ValidCard$ Creature,Planeswalker | ReplaceWith$ EnterAsCopy | Layer$ Copy | ReplacementResult$ Updated | Description$ The next time one or more creatures or planeswalkers enter this turn, they enter as copies of the chosen creature.",
	"SVar:EnterAsCopy:DB$ Clone | Defined$ Remembered | CloneTarget$ ReplacedCard | SubAbility$ DBImprint",
	"SVar:DBImprint:DB$ Pump | ImprintCards$ ReplacedCard",
	"SVar:TrigRemove:Mode$ ChangesZoneAll | CheckSVar$ Z | Execute$ ExileSelf | Static$ True",
	"SVar:ExileSelf:DB$ ChangeZone | Origin$ Command | Destination$ Exile | Defined$ Self",
	"SVar:Z:Imprinted$Amount",
}

// TestEffectHostedCopyReplacementCopiesTheWholeBatchOnce proves Mystic
// Reflection's "the next time one or more creatures enter, they enter as
// copies": two cards entering together (one ChangeZoneAll) both copy the
// chosen creature, the effect ends when the batch is done, and a creature
// entering later is untouched.
func TestEffectHostedCopyReplacementCopiesTheWholeBatchOnce(t *testing.T) {
	t.Parallel()

	g, p, other := newTwoPlayerGame(t)
	giant := g.NewCard(giantDef(t), other, engine.Battlefield)
	mirror := g.NewCard(copyTestDef(t, "Test Mirror", "Enchantment", "", "", mirrorLines...), p, engine.Battlefield)
	bear1 := g.NewCard(copyTestDef(t, "Test Bear", "Creature Elf", "2", "2"), p, engine.Graveyard)
	bear2 := g.NewCard(copyTestDef(t, "Test Bear", "Creature Elf", "2", "2"), p, engine.Graveyard)
	later := g.NewCard(copyTestDef(t, "Test Bear", "Creature Elf", "2", "2"), p, engine.Graveyard)
	reanimate := g.NewCard(copyTestDef(t, "Test Rise", "Enchantment", "", "",
		"A:AB$ ChangeZoneAll | Cost$ 0 | ChangeType$ Creature.YouOwn | Origin$ Graveyard | Destination$ Battlefield"),
		p, engine.Battlefield)
	sc := engine.NewScriptedController()
	mustActivate(t, g, p, sc, mirror, giant)
	if n := g.Zone(engine.Command, p).Len(); n != 1 {
		t.Fatalf("command zone holds %d cards after the effect, want 1", n)
	}

	// Two of the three bears enter together: take them out of the third.
	g.Move(later, engine.Exile, p)
	sc.QueueCardOrder([]engine.CardID{bear1, bear2})
	mustActivate(t, g, p, sc, reanimate)
	for _, id := range []engine.CardID{bear1, bear2} {
		c := g.Card(id)
		if c.Zone != engine.Battlefield || c.Def.Name != "Copied Giant" || !c.IsCopy() {
			t.Errorf("batch card %d is %q in %v (copy %v), want a copy of Copied Giant on the battlefield", id, c.Def.Name, c.Zone, c.IsCopy())
		}
	}
	if n := g.Zone(engine.Command, p).Len(); n != 0 {
		t.Errorf("command zone holds %d cards after the batch, want the effect gone", n)
	}

	g.Move(later, engine.Graveyard, p)
	mustActivate(t, g, p, sc, reanimate)
	if c := g.Card(later); c.Zone != engine.Battlefield || c.IsCopy() {
		t.Errorf("later entry is %q in %v (copy %v), want an uncopied creature", c.Def.Name, c.Zone, c.IsCopy())
	}
}

// TestEntersAsCopyLosesCardTypes proves Clone's RemoveCardTypes$ and
// RemoveSubTypes$ (Machine God's Effigy, Imposter Mech): the copy loses the
// copied card's card types before AddTypes$ lands, and with RemoveSubTypes$
// the subtypes no remaining type allows; RemoveSubTypes$ alone does nothing,
// as CardFactory.getCloneStates reads it only under RemoveCardTypes$.
func TestEntersAsCopyLosesCardTypes(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name                 string
		params               string
		creature, giantTypes bool
	}{
		{"card types only", "RemoveCardTypes$ True", false, true},
		{"card and sub types", "RemoveCardTypes$ True | RemoveSubTypes$ True", false, false},
		{"sub types alone", "RemoveSubTypes$ True", true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			g, p, other := newTwoPlayerGame(t)
			giant := g.NewCard(giantDef(t), other, engine.Battlefield)
			card := g.NewCard(testCloneDef(t, "Test Effigy", "Creature.Other | AddTypes$ Artifact | "+tc.params), p, engine.Hand)
			sc := engine.NewScriptedController()
			sc.QueueConfirmEffect(true)
			sc.QueueCardChoice([]engine.CardID{giant})
			mustCastAndResolve(t, g, p, sc, card)
			c := g.Card(card)
			if c.Def.Name != "Copied Giant" {
				t.Fatalf("entered as %q, want a copy of Copied Giant", c.Def.Name)
			}
			if got := c.Type().Has(cardtype.Creature); got != tc.creature {
				t.Errorf("creature = %v, want %v", got, tc.creature)
			}
			if !c.Type().Has(cardtype.Artifact) {
				t.Error("the added Artifact type is missing")
			}
			if got := c.Type().HasSubtype("Giant"); got != tc.giantTypes {
				t.Errorf("Giant subtype = %v, want %v", got, tc.giantTypes)
			}
			if !g.Card(giant).Type().Has(cardtype.Creature) {
				t.Error("the copied card lost its types")
			}
		})
	}
}

// TestEffectCreatedByTheCopyChainLeavesNothingBehind proves Spark Double's
// and Moritte's effect card ends with the entry it counted: the Command zone
// is empty afterwards, and the counters came in with the copy.
func TestEffectCreatedByTheCopyChainLeavesNothingBehind(t *testing.T) {
	t.Parallel()

	g, p, other := newTwoPlayerGame(t)
	giant := g.NewCard(giantDef(t), other, engine.Battlefield)
	g.NewCard(giantDef(t), p, engine.Battlefield)
	double := g.NewCard(copyTestDef(t, "Test Spark Double", "Creature Shapeshifter", "0", "0", "Cost:G",
		"K:ETBReplacement:Copy:DBCopy:Optional",
		"SVar:DBCopy:DB$ Clone | Choices$ Creature.Other+YouCtrl,Planeswalker.Other+YouCtrl | NonLegendary$ True | SubAbility$ DBConditionEffect",
		"SVar:DBConditionEffect:DB$ Effect | RememberObjects$ Self | ReplacementEffects$ ETBCreatPlans",
		"SVar:ETBCreatPlans:Event$ Moved | ValidCard$ Creature.IsRemembered,Planeswalker.IsRemembered | Destination$ Battlefield | ReplaceWith$ DBPutP1P1 | ReplacementResult$ Updated | Description$ x",
		"SVar:DBPutP1P1:DB$ PutCounter | Defined$ ReplacedNewCard.Creature | CounterType$ P1P1 | ETB$ True | CounterNum$ 1 | SubAbility$ DBPutLOYALTY",
		"SVar:DBPutLOYALTY:DB$ PutCounter | Defined$ ReplacedNewCard.Planeswalker | CounterType$ LOYALTY | ETB$ True | CounterNum$ 1 | SubAbility$ DBExile",
		"SVar:DBExile:DB$ ChangeZone | Defined$ Self | Origin$ Command | Destination$ Exile"), p, engine.Hand)
	_ = giant
	sc := engine.NewScriptedController()
	sc.QueueConfirmEffect(true)
	sc.QueueCardChoice([]engine.CardID{g.Zone(engine.Battlefield, p).Cards()[0]})
	mustCastAndResolve(t, g, p, sc, double)

	if got := counterCount(g, double, engine.P1P1); got != 1 {
		t.Errorf("+1/+1 counters = %d, want 1", got)
	}
	if got := counterCount(g, double, engine.Loyalty); got != 0 {
		t.Errorf("loyalty counters = %d, want 0 on a creature", got)
	}
	if n := g.Zone(engine.Command, p).Len(); n != 0 {
		t.Errorf("command zone holds %d cards, want the effect gone", n)
	}
}
