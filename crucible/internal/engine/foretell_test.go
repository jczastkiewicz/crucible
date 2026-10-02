package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
	"github.com/jczastkiewicz/crucible/internal/mana"
)

// CR 702.143: foretell is a special action ({2}, your turn) that exiles the
// card from your hand; from the next turn on only its owner may cast it, for
// its foretell cost (CastFromOwnZoneTest#onlyTheOwnerMayCastAForetoldCard,
// GameActionUtil.java:205-220).
func TestForetoldCardIsCastableOnlyByItsOwnerOnALaterTurn(t *testing.T) {
	t.Parallel()

	g, p, other := newTwoPlayerGameOn(t, scenarioDB(t))
	g.SetTurnState(1, p, engine.Main1)
	bolt := g.NewCard(corpusCard(t, "Demon Bolt"), p, engine.Hand)
	victim := g.NewCard(corpusCard(t, "Grizzly Bears"), other, engine.Battlefield)
	c := engine.NewScriptedController()

	g.Player(p).ManaPool.Add(mana.Red, 2)
	c.QueuePayGeneric(mana.ShardR)
	c.QueuePayGeneric(mana.ShardR)
	if !g.Foretell(p, bolt, c) {
		t.Fatal("could not foretell Demon Bolt")
	}
	if got := g.Card(bolt).Zone; got != engine.Exile {
		t.Fatalf("Demon Bolt is in %v after foretelling, want Exile", got)
	}

	// The turn it was foretold it cannot be cast.
	g.Player(p).ManaPool.Add(mana.Red, 1)
	if g.CastSpell(p, bolt, c) {
		t.Error("cast a card the turn it was foretold")
	}

	// A later turn: the other player may not cast it, its owner may.
	g.SetTurnState(2, other, engine.Main1)
	g.Player(other).ManaPool.Add(mana.Red, 3)
	if g.CastSpell(other, bolt, c) {
		t.Error("the other player cast a foretold card they do not own")
	}
	g.SetTurnState(3, p, engine.Main1)
	g.Player(p).ManaPool.Add(mana.Red, 1)
	c.QueueTargets([]engine.EntityID{engine.CardEntity(victim)})
	if !g.CastSpell(p, bolt, c) {
		t.Fatal("the owner could not cast the foretold card for {R}")
	}
	if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if got := g.Card(victim).Zone; got != engine.Graveyard {
		t.Errorf("Grizzly Bears is in %v, want Graveyard (4 damage)", got)
	}
	// Foretell has no exile replacement (only Flashback and Beam me up do).
	if got := g.Card(bolt).Zone; got != engine.Graveyard {
		t.Errorf("the foretold Demon Bolt is in %v after resolving, want Graveyard", got)
	}
}

// Foretelling needs a Foretell card in your hand, on your own turn, and {2}.
func TestForetellIsRefusedWithoutItsRequirements(t *testing.T) {
	t.Parallel()

	g, p, other := newTwoPlayerGameOn(t, scenarioDB(t))
	g.SetTurnState(1, p, engine.Main1)
	bolt := g.NewCard(corpusCard(t, "Demon Bolt"), p, engine.Hand)
	bears := g.NewCard(corpusCard(t, "Grizzly Bears"), p, engine.Hand)
	c := engine.NewScriptedController()
	g.Player(p).ManaPool.Add(mana.Red, 2)
	if g.Foretell(p, bears, c) {
		t.Error("foretold a card with no Foretell")
	}
	if g.Foretell(other, bolt, c) {
		t.Error("foretold another player's card")
	}
	g.SetTurnState(2, other, engine.Main1)
	if g.Foretell(p, bolt, c) {
		t.Error("foretold on another player's turn")
	}
}
