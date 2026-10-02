package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
	"github.com/jczastkiewicz/crucible/internal/mana"
)

// ReplacementHandlerTest#testPerpetualEntersTappedReplacementEffect: a card
// that perpetually gained "this permanent enters tapped" (Boareskyr
// Tollkeeper's Animate with Duration$ Perpetual and Replacements$) enters the
// battlefield tapped, once, from the grant row it kept while in hand.
func TestPerpetualEntersTappedReplacementTapsTheCardAsItEnters(t *testing.T) {
	t.Parallel()

	g, p, other := newTwoPlayerGameOn(t, scenarioDB(t))
	g.SetTurnState(1, p, engine.Main1)
	toll := g.NewCard(corpusCard(t, "Boareskyr Tollkeeper"), p, engine.Hand)
	bears := g.NewCard(corpusCard(t, "Grizzly Bears"), other, engine.Hand)
	g.Player(p).ManaPool.Add(mana.White, 2)
	c := engine.NewScriptedController()
	c.QueuePayGeneric(mana.ShardW)
	c.QueueTargets([]engine.EntityID{engine.PlayerEntity(other)})
	c.QueueCardChoice([]engine.CardID{bears})
	if !g.CastSpell(p, toll, c) {
		t.Fatal("could not cast Boareskyr Tollkeeper")
	}
	if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}

	g.SetTurnState(2, other, engine.Main1)
	g.Player(other).ManaPool.Add(mana.Green, 2)
	oc := engine.NewScriptedController()
	oc.QueuePayGeneric(mana.ShardG)
	if !g.CastSpell(other, bears, oc) {
		t.Fatal("could not cast Grizzly Bears")
	}
	if err := g.ResolveStack(engine.NewRegistry(), oc); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if got := g.Card(bears).Zone; got != engine.Battlefield {
		t.Fatalf("Grizzly Bears is in %v, want Battlefield", got)
	}
	if !g.Card(bears).Tapped {
		t.Error("the card with the perpetual enters-tapped replacement entered untapped")
	}
}

// A perpetual replacement nothing reads would be inert, so only the
// enters-tapped shape is granted; anything else fails when it is applied
// (GO-7).
func TestPerpetualReplacementOfAnotherShapeFailsClosed(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGameOn(t, scenarioDB(t))
	err := resolveWith(t, g, p, engine.NewScriptedController(),
		"DB$ Animate | Defined$ Self | Duration$ Perpetual | Replacements$ Rep",
		"Rep", "Event$ Moved | ValidCard$ Card.Self | Destination$ Graveyard | ReplaceWith$ Exiled",
		"Exiled", "DB$ ChangeZone | Defined$ ReplacedCard | Origin$ Graveyard | Destination$ Exile")
	if err == nil {
		t.Error("granted a perpetual replacement no consumer reads")
	}
}
