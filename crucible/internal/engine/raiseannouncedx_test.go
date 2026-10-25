package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
	"github.com/jczastkiewicz/crucible/internal/mana"
)

// Soul Immolation: "As an additional cost, blight X", X announced before the
// cost is paid and capped by XMax$ (the greatest toughness among your
// creatures). The blight is X -1/-1 counters on a creature you control; the
// damage is X to each opponent and each creature they control.
func TestSoulImmolationAnnouncesXForItsBlightCost(t *testing.T) {
	t.Parallel()

	g, p, other := newTwoPlayerGameOn(t, scenarioDB(t))
	c := engine.NewScriptedController()
	g.SetTurnState(1, p, engine.Main1)
	own := g.NewCard(corpusCard(t, "Grizzly Bears"), p, engine.Battlefield)
	g.NewCard(corpusCard(t, "Grizzly Bears"), other, engine.Battlefield)
	spell := g.NewCard(corpusCard(t, "Soul Immolation"), p, engine.Hand)
	g.Player(p).ManaPool.Add(mana.Red, 5)
	c.QueuePayX(2)
	for range 3 {
		c.QueuePayGeneric(mana.ShardR)
	}
	c.QueueCardChoice([]engine.CardID{own})
	if !g.CastSpell(p, spell, c) {
		t.Fatalf("CastSpell failed: %v", g.TakePendingError())
	}
	if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
		t.Fatalf("ResolveStack: %v (pending %v)", err, g.TakePendingError())
	}
	if got := g.Player(other).Life; got != 18 {
		t.Errorf("opponent life = %d, want 18 (X = 2)", got)
	}
}

// Waterbender's Restoration: "waterbend X" is announced with the spell, paid by
// tapping the caster's creatures for generic mana, and X targets are chosen
// after it (TargetMin$ X).
func TestWaterbendersRestorationTapsCreaturesForItsAnnouncedX(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGameOn(t, scenarioDB(t))
	c := engine.NewScriptedController()
	g.SetTurnState(1, p, engine.Main1)
	bears := g.NewCard(corpusCard(t, "Grizzly Bears"), p, engine.Battlefield)
	helper := g.NewCard(corpusCard(t, "Grizzly Bears"), p, engine.Battlefield)
	spell := g.NewCard(corpusCard(t, "Waterbender's Restoration"), p, engine.Hand)
	g.Player(p).ManaPool.Add(mana.Blue, 2)
	c.QueuePayX(1)
	c.QueueCardChoice([]engine.CardID{helper})
	c.QueueTargets([]engine.EntityID{engine.CardEntity(bears)})
	c.QueuePayGeneric(mana.ShardU)
	if !g.CastSpell(p, spell, c) {
		t.Fatalf("CastSpell failed: %v", g.TakePendingError())
	}
	if !g.Card(helper).Tapped {
		t.Errorf("the helper creature was not tapped for the waterbend")
	}
	if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
		t.Fatalf("ResolveStack: %v (pending %v)", err, g.TakePendingError())
	}
	if z := g.Card(bears).Zone; z != engine.Exile {
		t.Errorf("the targeted creature is in %v, want Exile", z)
	}
}
