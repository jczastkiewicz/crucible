package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
	"github.com/jczastkiewicz/crucible/internal/mana"
)

// castWithGreen casts card for p after adding n green mana, reporting
// whether the cast succeeded; generic costs are paid from the pool.
func castWithGreen(g *engine.Game, p engine.PlayerID, card engine.CardID, n int) bool {
	g.Player(p).ManaPool.Add(mana.Green, n)
	c := engine.NewScriptedController()
	for range n {
		c.QueuePayGeneric(mana.ShardG)
	}
	return g.CastSpell(p, card, c)
}

// ReduceCost and RaiseCost (CR 601.2f): "spells you cast cost {1} less",
// "noncreature spells cost {1} more", applied to a {2}{G} creature spell.
func TestSpellCostStatics(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name  string
		lines []string
		mana  int // mana needed beyond the {G}
		cost  int
	}{
		{"no static", nil, 0, 3},
		{"reduce by one", []string{"S:Mode$ ReduceCost | ValidCard$ Card | Type$ Spell | Activator$ You | Amount$ 1"}, 0, 2},
		{"reduce by two stacks", []string{
			"S:Mode$ ReduceCost | ValidCard$ Card | Type$ Spell | Activator$ You | Amount$ 1",
			"S:Mode$ ReduceCost | ValidCard$ Creature | Type$ Spell | Activator$ You | Amount$ 1"}, 0, 1},
		{"reduction never goes below the colored part", []string{"S:Mode$ ReduceCost | ValidCard$ Card | Type$ Spell | Amount$ 9"}, 0, 1},
		{"MinMana keeps the floor", []string{"S:Mode$ ReduceCost | ValidCard$ Card | Type$ Spell | Amount$ 9 | MinMana$ 2"}, 0, 2},
		{"raise by one", []string{"S:Mode$ RaiseCost | ValidCard$ Card | Type$ Spell | Amount$ 1"}, 0, 4},
		{"raise a noncreature does not touch a creature", []string{"S:Mode$ RaiseCost | ValidCard$ Card.nonCreature | Type$ Spell | Amount$ 1"}, 0, 3},
		{"raise then reduce", []string{
			"S:Mode$ RaiseCost | ValidCard$ Card | Type$ Spell | Amount$ 2",
			"S:Mode$ ReduceCost | ValidCard$ Card | Type$ Spell | Amount$ 1"}, 0, 4},
		{"opponent's reduction does not apply to me", []string{"S:Mode$ ReduceCost | ValidCard$ Card | Type$ Spell | Activator$ Opponent | Amount$ 1"}, 0, 3},
		{"an unresolvable line is not applied", []string{"S:Mode$ ReduceCost | ValidCard$ Card | Type$ Spell | Amount$ 1 | ValidTarget$ Creature.tapped"}, 0, 3},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			for _, extra := range []int{-1, 0} {
				g, p, _ := newTwoPlayerGame(t)
				for _, line := range tc.lines {
					g.NewCard(scriptDef(t, "Test Rule", "Enchantment", line), p, engine.Battlefield)
				}
				card := g.NewCard(creatureDefCost(t, "Costly", "2 G"), p, engine.Hand)
				have := tc.cost + extra
				if got := castWithGreen(g, p, card, have); got != (extra == 0) {
					t.Errorf("casting a {2}{G} spell with %d mana = %v, want %v (cost %d)", have, got, extra == 0, tc.cost)
				}
			}
		})
	}
}

// A spell's own "this spell costs {N} less" (EffectZone$ All, Amount$ an SVar of
// its own face) applies from hand: Count$ amounts resolve against the host.
func TestSelfReduceCostFromHand(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGame(t)
	def := scriptDef(t, "Test Affinity", "Creature Elf",
		"S:Mode$ ReduceCost | ValidCard$ Card.Self | Type$ Spell | Amount$ X | EffectZone$ All",
		"SVar:X:Count$Valid Creature.YouCtrl")
	def.Faces[0].ManaCost = mana.MustParse("3 G")
	g.NewCard(creatureDefPT(t, "1", "1"), p, engine.Battlefield)
	g.NewCard(creatureDefPT(t, "1", "1"), p, engine.Battlefield)
	card := g.NewCard(def, p, engine.Hand)
	if !castWithGreen(g, p, card, 2) {
		t.Error("a {3}{G} spell with two creatures in play (reduction 2) was not castable for 2 mana")
	}
}

// Affinity for artifacts (CR 702.41a) expands to a self ReduceCost whose
// amount counts the caster's artifacts: Frogmite ({4}) costs {2} with two.
func TestAffinityReducesTheCost(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGameOn(t, scenarioDB(t))
	g.NewCard(corpusCard(t, "Bonesplitter"), p, engine.Battlefield)
	g.NewCard(corpusCard(t, "Bonesplitter"), p, engine.Battlefield)
	frog := g.NewCard(corpusCard(t, "Frogmite"), p, engine.Hand)
	if !castWithGreen(g, p, frog, 2) {
		t.Error("Frogmite with two artifacts in play was not castable for 2 mana")
	}
}

// Convoke (CR 702.51a): each creature tapped pays {1} or one mana of its color.
// Siege Wurm ({5}{G}{G}): two green creatures pay the {G}{G}, leaving {5}.
func TestConvokeTapsCreaturesToPay(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGameOn(t, scenarioDB(t))
	wurm := g.NewCard(corpusCard(t, "Siege Wurm"), p, engine.Hand)
	first := g.NewCard(corpusCard(t, "Grizzly Bears"), p, engine.Battlefield)
	second := g.NewCard(corpusCard(t, "Grizzly Bears"), p, engine.Battlefield)
	g.Player(p).ManaPool.Add(mana.Red, 5)
	c := engine.NewScriptedController()
	c.QueueCardChoice([]engine.CardID{first, second})
	for range 5 {
		c.QueuePayGeneric(mana.ShardR)
	}
	if !g.CastSpell(p, wurm, c) {
		t.Fatal("Siege Wurm could not be cast with two convoking creatures and five mana")
	}
	if !g.Card(first).Tapped || !g.Card(second).Tapped {
		t.Error("the convoking creatures were not tapped")
	}
}

// Delve (CR 702.66a): each card exiled from the graveyard pays {1}.
func TestDelveExilesCardsToPay(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGameOn(t, scenarioDB(t))
	cruise := g.NewCard(corpusCard(t, "Treasure Cruise"), p, engine.Hand)
	var grave []engine.CardID
	for range 4 {
		grave = append(grave, g.NewCard(creatureDefPT(t, "1", "1"), p, engine.Graveyard))
	}
	g.Player(p).ManaPool.Add(mana.Blue, 4)
	c := engine.NewScriptedController()
	c.QueueCardChoice(grave)
	for range 3 {
		c.QueuePayGeneric(mana.ShardU)
	}
	if !g.CastSpell(p, cruise, c) {
		t.Fatal("Treasure Cruise could not be cast delving four cards")
	}
	for _, id := range grave {
		if g.Card(id).Zone != engine.Exile {
			t.Errorf("a delved card is in %v, want Exile", g.Card(id).Zone)
		}
	}
}

// A convoke cast that cannot be paid taps nothing.
func TestConvokeTapsNothingWhenTheCastFails(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGameOn(t, scenarioDB(t))
	wurm := g.NewCard(corpusCard(t, "Siege Wurm"), p, engine.Hand)
	bear := g.NewCard(corpusCard(t, "Grizzly Bears"), p, engine.Battlefield)
	g.Player(p).ManaPool.Add(mana.Green, 1)
	c := engine.NewScriptedController()
	c.QueueCardChoice([]engine.CardID{bear})
	for range 6 {
		c.QueuePayGeneric(mana.ShardR)
	}
	if g.CastSpell(p, wurm, c) {
		t.Fatal("Siege Wurm was cast with no mana")
	}
	if g.Card(bear).Tapped {
		t.Error("a creature stayed tapped after the cast failed")
	}
}

// An instant's A:SP$ Cost$ is an additional cost (CR 118.8): Village Rites
// ({B}, sacrifice a creature) cannot be cast without a creature to sacrifice,
// and sacrifices the chosen one when it is.
func TestSpellAdditionalCostIsPaid(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGameOn(t, scenarioDB(t))
	rites := g.NewCard(corpusCard(t, "Village Rites"), p, engine.Hand)
	g.NewCard(creatureDefPT(t, "1", "1"), p, engine.Library)
	g.NewCard(creatureDefPT(t, "1", "1"), p, engine.Library)
	g.Player(p).ManaPool.Add(mana.Black, 1)
	c := engine.NewScriptedController()
	if g.CastSpell(p, rites, c) {
		t.Fatal("Village Rites was cast with no creature to sacrifice")
	}
	fodder := g.NewCard(creatureDefPT(t, "1", "1"), p, engine.Battlefield)
	c.QueueSacrificeChoice([]engine.CardID{fodder})
	if !g.CastSpell(p, rites, c) {
		t.Fatal("Village Rites could not be cast with a creature to sacrifice")
	}
	if g.Card(fodder).Zone != engine.Graveyard {
		t.Errorf("the sacrificed creature is in %v, want Graveyard", g.Card(fodder).Zone)
	}
	if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if got := len(g.Zone(engine.Hand, p).Cards()); got != 2 {
		t.Errorf("cards in hand = %d, want 2 drawn", got)
	}
}

// A permanent spell's A:SP$ PermanentCreature | Cost$ line is its additional
// cost too ("as an additional cost to cast this spell, discard a card").
func TestPermanentSpellAdditionalCostIsPaid(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGame(t)
	def := scriptDef(t, "Test Discarder", "Creature Elf", "A:SP$ PermanentCreature | Cost$ 1 B Discard<1/Card>")
	def.Faces[0].ManaCost = mana.MustParse("1 B")
	card := g.NewCard(def, p, engine.Hand)
	g.Player(p).ManaPool.Add(mana.Black, 2)
	c := engine.NewScriptedController()
	c.QueuePayGeneric(mana.ShardB)
	if g.CastSpell(p, card, c) {
		t.Fatal("the creature was cast with no card to discard")
	}
	pitch := g.NewCard(creatureDefPT(t, "1", "1"), p, engine.Hand)
	c.QueueDiscardChoice([]engine.CardID{pitch})
	if !g.CastSpell(p, card, c) {
		t.Fatal("the creature could not be cast with a card to discard")
	}
	if g.Card(pitch).Zone != engine.Graveyard {
		t.Errorf("the discarded card is in %v, want Graveyard", g.Card(pitch).Zone)
	}
}

// A RaiseCost this port cannot evaluate refuses the cast rather than casting
// for less than Java charges (GO-7).
func TestUnresolvableRaiseCostRefusesTheCast(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGame(t)
	g.NewCard(scriptDef(t, "Test Tax", "Enchantment",
		"S:Mode$ RaiseCost | ValidCard$ Card | Type$ Spell | Amount$ 1 | ValidTarget$ Creature"), p, engine.Battlefield)
	card := g.NewCard(creatureDefCost(t, "Costly", "2 G"), p, engine.Hand)
	if castWithGreen(g, p, card, 9) {
		t.Error("a spell was cast under a RaiseCost the engine cannot evaluate")
	}
}
