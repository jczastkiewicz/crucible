package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
	"github.com/jczastkiewicz/crucible/internal/valid"
)

// TestSharesColorWithRestrictions covers CardProperty's suffixed
// SharesColorWith forms (CardProperty.java:636-691): a colorless card shares
// none, Imprinted compares against the source's imprinted cards,
// MostProminentColor against the battlefield's commonest colors (every color
// tied when none is colored, PORT-7), and an unknown restriction is a pending
// error, never false-by-guess (GO-7).
func TestSharesColorWithRestrictions(t *testing.T) {
	t.Parallel()

	g, p, other := newTwoPlayerGameOn(t, scenarioDB(t))
	green := g.NewCard(corpusCard(t, "Grizzly Bears"), p, engine.Battlefield)
	red := g.NewCard(corpusCard(t, "Lightning Bolt"), other, engine.Hand)
	ring := g.NewCard(corpusCard(t, "Sol Ring"), p, engine.Battlefield)
	matches := func(c engine.CardID, spec string) bool {
		return engine.Matches(g, g.Card(c), valid.Parse(spec), p, green)
	}

	if matches(ring, "Card.SharesColorWith Imprinted") {
		t.Error("a colorless card shares a color with something")
	}
	if matches(red, "Card.SharesColorWith Imprinted") {
		t.Error("nothing imprinted, yet a red card shares a color")
	}
	g.Card(green).Memory.Imprint(red)
	if !matches(red, "Card.SharesColorWith Imprinted") {
		t.Error("the imprinted red card does not share red with itself")
	}
	if matches(ring, "Card.SharesColorWith Imprinted") {
		t.Error("colorless Sol Ring shares a color with the imprinted card")
	}

	// Green is the only colored permanent: the most prominent color.
	if !matches(green, "Card.SharesColorWith MostProminentColor") {
		t.Error("the green permanent does not share the most prominent color")
	}
	if matches(red, "Card.SharesColorWith MostProminentColor") {
		t.Error("red shares the most prominent color of a green battlefield")
	}

	if matches(green, "Card.SharesColorWith Bogus") {
		t.Error("an unknown restriction matched")
	}
	if err := g.TakePendingError(); err == nil {
		t.Error("an unknown SharesColorWith restriction recorded no pending error")
	}
	if matches(green, "Card.SharesColorWith ActivationColor") {
		t.Error("ActivationColor matched with no resolving ability")
	}
}

// TestMostProminentColorTiesEveryColorOnAColorlessBattlefield is PORT-7:
// nMax starts at -1, so with no colored permanent all five colors tie at 0.
func TestMostProminentColorTiesEveryColorOnAColorlessBattlefield(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGameOn(t, scenarioDB(t))
	ring := g.NewCard(corpusCard(t, "Sol Ring"), p, engine.Battlefield)
	green := g.NewCard(corpusCard(t, "Grizzly Bears"), p, engine.Hand)
	if !engine.Matches(g, g.Card(green), valid.Parse("Card.SharesColorWith MostProminentColor"), p, ring) {
		t.Error("with no colored permanent every color ties, so a green card shares one")
	}
}
