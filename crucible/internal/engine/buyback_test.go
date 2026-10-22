package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
	"github.com/jczastkiewicz/crucible/internal/mana"
)

// TestBuybackReturnsTheSpellToHandAsItResolves proves K:Buyback (CR 702.27):
// paying the optional cost puts the resolved spell into its owner's hand,
// declining it leaves the spell in the graveyard.
func TestBuybackReturnsTheSpellToHandAsItResolves(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name    string
		pay     bool
		want    engine.ZoneType
		wantLif int
	}{
		{"Paid", true, engine.Hand, 21},
		{"Declined", false, engine.Graveyard, 21},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			g, p, _ := newTwoPlayerGame(t)
			def := spellDefWith(t, "Test Buyback Gain", "Instant", "G", "SP$ GainLife | Defined$ You | LifeAmount$ 1")
			def.Faces[0].Keywords = append(def.Faces[0].Keywords, "Buyback:2")
			spell := g.NewCard(def, p, engine.Hand)
			g.Player(p).ManaPool.Add(mana.Green, 3)
			c := engine.NewScriptedController()
			c.QueueConfirmPayCost(tc.pay)
			if tc.pay {
				c.QueuePayGeneric(mana.ShardG)
				c.QueuePayGeneric(mana.ShardG)
			}
			if !g.CastSpell(p, spell, c) {
				t.Fatal("could not cast the buyback spell")
			}
			if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
				t.Fatalf("ResolveStack: %v", err)
			}
			if got := g.Card(spell).Zone; got != tc.want {
				t.Errorf("spell in %v, want %v", got, tc.want)
			}
			if got := g.Player(p).Life; got != tc.wantLif {
				t.Errorf("life = %d, want %d", got, tc.wantLif)
			}
		})
	}
}
