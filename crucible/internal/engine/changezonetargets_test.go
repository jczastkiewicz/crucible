package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
	"github.com/jczastkiewicz/crucible/internal/mana"
)

// castRaiseDead casts Raise Dead from p's hand with {B} at target, runs leave
// (if set) while the spell is on the stack, then resolves it.
func castRaiseDead(t *testing.T, g *engine.Game, p engine.PlayerID, target engine.CardID, leave func()) {
	t.Helper()
	spell := g.NewCard(corpusCard(t, "Raise Dead"), p, engine.Hand)
	c := engine.NewScriptedController()
	c.QueueTargets([]engine.EntityID{engine.CardEntity(target)})
	g.Player(p).ManaPool.Add(mana.Black, 1)
	if !g.CastSpell(p, spell, c) {
		t.Fatal("CastSpell failed: Raise Dead should be able to target a creature card in the graveyard")
	}
	if leave != nil {
		leave()
	}
	if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
}

// A ChangeZone line with Origin$ and no player target aims at cards in its
// Origin$ zones (AbilityFactory.adjustChangeZoneTarget), so Raise Dead is
// castable at a graveyard creature and returns it to hand.
func TestChangeZoneTargetsCardsInItsOriginZone(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGame(t)
	bear := g.NewCard(corpusCard(t, "Grizzly Bears"), p, engine.Graveyard)
	castRaiseDead(t, g, p, bear, nil)

	if got := g.Card(bear).Zone; got != engine.Hand {
		t.Errorf("Grizzly Bears zone = %v, want Hand", got)
	}
}

// The target is checked again on resolution: a card that left the graveyard
// since the cast is no longer a legal target, so the spell fizzles and moves
// nothing (CR 608.2b).
func TestChangeZoneFizzlesWhenTheTargetLeavesItsOriginZone(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGame(t)
	bear := g.NewCard(corpusCard(t, "Grizzly Bears"), p, engine.Graveyard)
	castRaiseDead(t, g, p, bear, func() { g.Move(bear, engine.Exile, p) })

	if got := g.Card(bear).Zone; got != engine.Exile {
		t.Errorf("Grizzly Bears zone = %v, want Exile -- a fizzled Raise Dead must not move it", got)
	}
}

// A battlefield creature is not a legal Raise Dead target: Origin$ replaces
// the battlefield default instead of widening it.
func TestChangeZoneDoesNotTargetTheBattlefieldWhenOriginIsGraveyard(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGame(t)
	bear := g.NewCard(corpusCard(t, "Grizzly Bears"), p, engine.Battlefield)
	spell := g.NewCard(corpusCard(t, "Raise Dead"), p, engine.Hand)
	c := engine.NewScriptedController()
	c.QueueTargets([]engine.EntityID{engine.CardEntity(bear)})
	g.Player(p).ManaPool.Add(mana.Black, 1)
	if g.CastSpell(p, spell, c) {
		t.Error("CastSpell succeeded with only a battlefield creature; Raise Dead needs a graveyard target")
	}
}
