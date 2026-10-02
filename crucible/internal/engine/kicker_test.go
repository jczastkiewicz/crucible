package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
	"github.com/jczastkiewicz/crucible/internal/mana"
	"github.com/jczastkiewicz/crucible/internal/valid"
)

// Kicker (CR 702.33a): Burst Lightning ({R}, kicker {4}) deals 2 damage, or 4
// when kicked; the caster is asked and pays the extra cost only on yes.
func TestKickerAddsItsCostAndChangesTheEffect(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name   string
		kick   bool
		reds   int
		wantOK bool
		damage int
	}{
		{"unkicked", false, 1, true, 2},
		{"kicked", true, 5, true, 4},
		{"kicked without the mana", true, 1, false, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			g, p, other := newTwoPlayerGameOn(t, scenarioDB(t))
			bolt := g.NewCard(corpusCard(t, "Burst Lightning"), p, engine.Hand)
			g.Player(p).ManaPool.Add(mana.Red, tc.reds)
			c := engine.NewScriptedController()
			c.QueueConfirmPayCost(tc.kick)
			for range 4 {
				c.QueuePayGeneric(mana.ShardR)
			}
			c.QueueTargets([]engine.EntityID{engine.PlayerEntity(other)})
			if got := g.CastSpell(p, bolt, c); got != tc.wantOK {
				t.Fatalf("CastSpell = %v, want %v", got, tc.wantOK)
			}
			if !tc.wantOK {
				return
			}
			if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
				t.Fatalf("ResolveStack: %v", err)
			}
			if got := 20 - g.Player(other).Life; got != tc.damage {
				t.Errorf("damage dealt = %d, want %d", got, tc.damage)
			}
		})
	}
}

// A permanent keeps its kicker choice: "if CARDNAME was kicked, it enters with
// +1/+1 counters" reads the `kicked` property and Count$Kicked on the battlefield.
func TestKickedPermanentRemembersItOnTheBattlefield(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGame(t)
	def := scriptDef(t, "Test Kicker Beast", "Creature Elf", "K:Kicker:G")
	def.Faces[0].ManaCost = mana.MustParse("G")
	card := g.NewCard(def, p, engine.Hand)
	g.Player(p).ManaPool.Add(mana.Green, 2)
	c := engine.NewScriptedController()
	c.QueueConfirmPayCost(true)
	if !g.CastSpell(p, card, c) {
		t.Fatal("CastSpell failed")
	}
	if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if !engine.Matches(g, g.Card(card), valid.Parse("Card.Self+kicked"), p, card) {
		t.Error("the kicked creature on the battlefield does not match the kicked property")
	}
}
