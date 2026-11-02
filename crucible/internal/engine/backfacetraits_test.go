package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
	"github.com/jczastkiewicz/crucible/internal/mana"
)

// A transforming card's statics are its current face's only
// (Card.getStaticAbilities reads currentState): Heliod, the Radiant Dawn grants
// no flash, Heliod, the Warped Eclipse, its back face, does ("You may cast
// spells as though they had flash").
func TestBackFaceStaticsApplyOnlyOnceTransformed(t *testing.T) {
	t.Parallel()
	g, p, opp := newTwoPlayerGameOn(t, scenarioDB(t))
	g.SetTurnState(1, opp, engine.Main1)
	heliod := g.NewCard(corpusCard(t, "Heliod, the Radiant Dawn"), p, engine.Battlefield)
	bears := g.NewCard(corpusCard(t, "Grizzly Bears"), p, engine.Hand)
	g.Player(p).ManaPool.Add(mana.Green, 2)
	c := engine.NewScriptedController()
	if g.CastSpell(p, bears, c) {
		t.Fatal("Grizzly Bears cast on the opponent's turn under Heliod's front face")
	}
	_ = g.TakePendingError()
	if !g.Transform(heliod) {
		t.Fatal("Heliod did not transform")
	}
	c.QueuePayGeneric(mana.ShardG)
	if !g.CastSpell(p, bears, c) {
		t.Fatalf("Grizzly Bears not cast with Heliod, the Warped Eclipse's flash: %v", g.TakePendingError())
	}
}

// A modal card's replacements are its current face's too: Blackbloom Rogue
// enters untapped, though Blackbloom Bog, its land face, enters tapped.
func TestFrontFaceEntersWithoutItsBackFacesReplacement(t *testing.T) {
	t.Parallel()
	g, p, _ := newTwoPlayerGameOn(t, scenarioDB(t))
	g.SetTurnState(1, p, engine.Main1)
	rogue := g.NewCard(corpusCard(t, "Blackbloom Rogue"), p, engine.Hand)
	g.Player(p).ManaPool.Add(mana.Black, 3)
	c := engine.NewScriptedController()
	c.QueuePayGeneric(mana.ShardB)
	c.QueuePayGeneric(mana.ShardB)
	if !g.CastSpell(p, rogue, c) {
		t.Fatalf("Blackbloom Rogue not cast: %v", g.TakePendingError())
	}
	if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
		t.Fatal(err)
	}
	if r := g.Card(rogue); r.Zone != engine.Battlefield || r.Tapped {
		t.Errorf("Blackbloom Rogue in %v, tapped %v, want untapped on the battlefield", r.Zone, r.Tapped)
	}
}

// Played as its land face, the card is Blackbloom Bog and its own
// replacement applies: it enters tapped.
func TestBackFacePlayedEntersWithItsOwnReplacement(t *testing.T) {
	t.Parallel()
	g, p, _ := newTwoPlayerGameOn(t, scenarioDB(t))
	g.SetTurnState(1, p, engine.Main1)
	card := g.NewCard(corpusCard(t, "Blackbloom Rogue"), p, engine.Hand)
	if !g.PlayLandFace(p, card, engine.NewScriptedController()) {
		t.Fatalf("Blackbloom Bog not played: %v", g.TakePendingError())
	}
	if c := g.Card(card); c.Zone != engine.Battlefield || !c.Tapped || c.Name() != "Blackbloom Bog" {
		t.Errorf("%q in %v, tapped %v, want Blackbloom Bog tapped on the battlefield", c.Name(), c.Zone, c.Tapped)
	}
}
