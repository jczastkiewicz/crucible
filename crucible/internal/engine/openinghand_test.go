package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
)

// MayEffectFromOpeningHand: a Leyline-style card the player chooses goes onto
// the battlefield with no stack; a card left unchosen stays in hand.
func TestOpeningHandActionsPutChosenCardOntoBattlefield(t *testing.T) {
	t.Parallel()

	g, p, other := newTwoPlayerGame(t)
	def := scriptDef(t, "Test Leyline", "Enchantment",
		"K:MayEffectFromOpeningHand:FromHand",
		"SVar:FromHand:DB$ ChangeZone | Defined$ Self | Origin$ Hand | Destination$ Battlefield")
	chosen := g.NewCard(def, p, engine.Hand)
	left := g.NewCard(def, p, engine.Hand)
	c := engine.NewScriptedController()
	c.QueueCardChoice([]engine.CardID{chosen})

	first, err := engine.RunOpeningHandActions(g, engine.NewRegistry(), c, p)
	if err != nil {
		t.Fatalf("RunOpeningHandActions: %v", err)
	}

	if first != p {
		t.Errorf("first = %v, want %v", first, p)
	}
	if got := g.Card(chosen).Zone; got != engine.Battlefield {
		t.Errorf("chosen card zone = %v, want Battlefield", got)
	}
	if got := g.Card(left).Zone; got != engine.Hand {
		t.Errorf("unchosen card zone = %v, want Hand", got)
	}
	_ = other
}

// Impatient Iguana: BecomeStartingPlayer$ on the revealed card makes its
// player the first one, but !PlayFirst keeps the card from the player who
// was first.
func TestOpeningHandActionsBecomeStartingPlayer(t *testing.T) {
	t.Parallel()

	g, p, other := newTwoPlayerGame(t)
	def := scriptDef(t, "Test Iguana", "Creature Lizard",
		"K:MayEffectFromOpeningHand:RevealCard:!PlayFirst",
		"SVar:RevealCard:DB$ Reveal | RevealDefined$ Self | BecomeStartingPlayer$ True")
	g.NewCard(def, p, engine.Hand)
	mine := g.NewCard(def, other, engine.Hand)
	c := engine.NewScriptedController()
	c.QueueCardChoice([]engine.CardID{mine})

	first, err := engine.RunOpeningHandActions(g, engine.NewRegistry(), c, p)
	if err != nil {
		t.Fatalf("RunOpeningHandActions: %v", err)
	}

	if first != other {
		t.Errorf("first = %v, want %v (the Iguana's player)", first, other)
	}
}
