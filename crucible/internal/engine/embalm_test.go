package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/cardtype"
	"github.com/jczastkiewicz/crucible/internal/engine"
	"github.com/jczastkiewicz/crucible/internal/mana"
)

// K:Embalm and K:Eternalize (CardFactoryUtil.java:2822-2834, :2914-2941): from
// the graveyard, pay the cost and exile the card to create a token copy with
// no mana cost, a white Zombie (Embalm) or a black 4/4 Zombie (Eternalize).
func TestEmbalmAndEternalizeMakeZombieTokenCopies(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		card, cost string
		color      mana.Colors
		power      int
	}{
		{"Anointer Priest", "3 W", mana.White, 1},
		{"Adorned Pouncer", "3 W W", mana.Black, 4},
	} {
		t.Run(tc.card, func(t *testing.T) {
			t.Parallel()
			g, p, _ := newTwoPlayerGameOn(t, scenarioDB(t))
			card := g.NewCard(corpusCard(t, tc.card), p, engine.Graveyard)
			c := engine.NewScriptedController()
			m, err := mana.Parse(tc.cost)
			if err != nil {
				t.Fatal(err)
			}
			g.Player(p).ManaPool.Add(mana.White, m.CMC())
			for i := 0; i < m.Generic(); i++ {
				c.QueuePayGeneric(mana.ShardW)
			}
			if !g.ActivateAbility(p, card, 0, c) {
				t.Fatalf("%s's ability could not be activated from the graveyard", tc.card)
			}
			if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
				t.Fatal(err)
			}
			if got := g.Card(card).Zone; got != engine.Exile {
				t.Errorf("%s is in %v, want exile", tc.card, got)
			}
			var token *engine.Card
			for _, id := range g.Zone(engine.Battlefield, p).Cards() {
				if g.Card(id).IsToken {
					token = g.Card(id)
				}
			}
			if token == nil {
				t.Fatal("no token copy entered")
			}
			if token.Name() != tc.card || token.Colors() != tc.color || !token.Type().HasSubtype("Zombie") || !token.Type().Has(cardtype.Creature) {
				t.Errorf("token %q colors %v types %v, want a %v Zombie copy", token.Name(), token.Colors(), token.Type(), tc.color)
			}
			if pw, _ := token.Power(); pw != tc.power {
				t.Errorf("token power %d, want %d", pw, tc.power)
			}
			if got := token.CMC(); got != 0 {
				t.Errorf("token mana value %d, want 0 (no mana cost)", got)
			}
		})
	}
}
