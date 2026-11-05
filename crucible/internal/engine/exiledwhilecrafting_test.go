package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
	"github.com/jczastkiewicz/crucible/internal/mana"
)

// Market Gnome's Mode$ Exiled names WhileKeyword$ Ability.Craft
// (Trigger.whileKeywordCheck, Trigger.java:644-656): exiled from the
// battlefield as the material of a Craft ability's cost, it gains 1 life and
// draws a card. Its trigger goes on the stack above the Craft ability.
func TestMarketGnomeExiledWhileCraftingGainsLifeAndDraws(t *testing.T) {
	t.Parallel()
	g, p, _, net, _ := craftBraidedNet(t, false)
	gnome := g.NewCard(corpusCard(t, "Market Gnome"), p, engine.Battlefield)
	g.NewCard(corpusCard(t, "Grizzly Bears"), p, engine.Library)
	c := engine.NewScriptedController()
	g.Player(p).ManaPool.Add(mana.Blue, 2)
	c.QueuePayGeneric(mana.ShardU)
	c.QueueCardChoice([]engine.CardID{gnome})
	if !g.ActivateAbility(p, net, craftIndex(g, net), c) {
		t.Fatalf("Craft: %v", g.TakePendingError())
	}
	if z := g.Card(gnome).Zone; z != engine.Exile {
		t.Fatalf("Market Gnome in %v, want Exile", z)
	}
	if n := g.StackLen(); n != 2 {
		t.Fatalf("stack holds %d objects, want the Craft ability and Market Gnome's trigger", n)
	}
	if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
		t.Fatal(err)
	}
	if life, hand := g.Player(p).Life, g.Zone(engine.Hand, p).Len(); life != 21 || hand != 1 {
		t.Errorf("life %d, hand %d, want 21 and 1", life, hand)
	}
}
