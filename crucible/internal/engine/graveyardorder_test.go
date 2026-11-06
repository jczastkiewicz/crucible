package engine_test

import (
	"slices"
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
)

// CR 613.7m (GameActionUtil.orderCardsByTheirOwners): cards going to the
// graveyard together are put there in the order their owner chooses. Each
// case moves two or three of p's cards and answers the order reversed; the
// graveyard holds them in that order.
func TestCardsMovingTogetherGoInTheOwnersOrder(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		run  func(t *testing.T, g *engine.Game, p engine.PlayerID, c *engine.ScriptedController, cards []engine.CardID)
		zone engine.ZoneType // where cards start
	}{
		{"mill", func(t *testing.T, g *engine.Game, p engine.PlayerID, c *engine.ScriptedController, _ []engine.CardID) {
			resolveLine(t, g, p, c, "DB$ Mill | Defined$ You | NumCards$ 3")
		}, engine.Library},
		{"destroy all", func(t *testing.T, g *engine.Game, p engine.PlayerID, c *engine.ScriptedController, _ []engine.CardID) {
			resolveLine(t, g, p, c, "DB$ DestroyAll | ValidCards$ Creature.powerEQ3")
		}, engine.Battlefield},
		{"destroy targets", func(t *testing.T, g *engine.Game, p engine.PlayerID, c *engine.ScriptedController, cards []engine.CardID) {
			targets := make([]engine.EntityID, len(cards))
			for i, id := range cards {
				targets[i] = engine.CardEntity(id)
			}
			if _, err := resolveNow(t, g, p, c, targets, "DB$ Destroy | ValidTgts$ Creature | TargetMin$ 3 | TargetMax$ 3"); err != nil {
				t.Fatal(err)
			}
		}, engine.Battlefield},
		{"lethal damage", func(_ *testing.T, g *engine.Game, _ engine.PlayerID, c *engine.ScriptedController, cards []engine.CardID) {
			for _, id := range cards {
				g.Card(id).Damage.Marked = 3
			}
			engine.CheckStateBasedActions(g, c)
		}, engine.Battlefield},
		{"sacrifice", func(t *testing.T, g *engine.Game, p engine.PlayerID, c *engine.ScriptedController, cards []engine.CardID) {
			c.QueueSacrificeChoice(cards)
			resolveLine(t, g, p, c, "DB$ Sacrifice | Defined$ You | SacValid$ Creature.powerEQ3 | Amount$ 3")
		}, engine.Battlefield},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			g, p, _ := newTwoPlayerGame(t)
			g.SetTurnState(1, p, engine.Main1)
			var cards []engine.CardID
			for range 3 {
				cards = append(cards, g.NewCard(creatureDefPT(t, "3", "3"), p, tc.zone))
			}
			reversed := slices.Clone(cards)
			slices.Reverse(reversed)
			c := engine.NewScriptedController()
			c.QueueCardOrder(reversed)
			tc.run(t, g, p, c, cards)
			if got := g.Zone(engine.Graveyard, p).Cards(); !slices.Equal(got, reversed) {
				t.Errorf("graveyard %v, want the chosen order %v", got, reversed)
			}
		})
	}
}
