package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
)

// Flipping is one-way (CR 709.4): a flip card shows its flipped face, a
// second flip does nothing, and leaving the battlefield restores the card.
func TestFlipIsOneWayAndEndsOnLeavingTheBattlefield(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGameOn(t, scenarioDB(t))
	id := g.NewCard(corpusCard(t, "Akki Lavarunner"), p, engine.Battlefield)
	if g.Card(id).InFlippedState() {
		t.Fatal("flipped before anything flipped it")
	}
	if !g.Flip(id) {
		t.Fatal("first flip reported no change")
	}
	c := g.Card(id)
	if !c.IsFlipped() || !c.InFlippedState() || c.Def.Name != "Tok-Tok, Volcano Born" {
		t.Errorf("after flip: flipped=%v state=%v name=%q", c.IsFlipped(), c.InFlippedState(), c.Def.Name)
	}
	if g.Flip(id) {
		t.Error("a flipped permanent flipped again")
	}
	g.Move(id, engine.Hand, p)
	c = g.Card(id)
	if c.IsFlipped() || c.Def.Name != "Akki Lavarunner" {
		t.Errorf("in hand: flipped=%v name=%q, want the unflipped card", c.IsFlipped(), c.Def.Name)
	}
}

// A card with no flipped face still flips rules-wise (Card.java:737-741): the
// flag is set and nothing else changes.
func TestFlipOfACardWithoutAFlippedFaceOnlySetsTheFlag(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGameOn(t, scenarioDB(t))
	id := g.NewCard(corpusCard(t, "Silvercoat Lion"), p, engine.Battlefield)
	if !g.Flip(id) {
		t.Fatal("flip reported no change")
	}
	c := g.Card(id)
	if !c.IsFlipped() || c.InFlippedState() || c.Def.Name != "Silvercoat Lion" {
		t.Errorf("flipped=%v state=%v name=%q", c.IsFlipped(), c.InFlippedState(), c.Def.Name)
	}
}
