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
