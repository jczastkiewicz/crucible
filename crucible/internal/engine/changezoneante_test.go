package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
)

// Ante is a plain destination (Jeweled Bird, Demonic Attorney, Rebirth): the
// move has no Ante branch in Java (ChangeZoneEffect, DigEffect.java:407), so
// the card lands in its owner's Ante zone and ChangeZoneAll can send it on.
func TestChangeZoneAnteRoundTrip(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGame(t)
	top := g.NewCard(creatureDefPT(t, "2", "2"), p, engine.Library)
	c := engine.NewScriptedController()
	if _, err := resolveNow(t, g, p, c, nil, "DB$ Dig | Defined$ You | DigNum$ 1 | ChangeNum$ All | DestinationZone$ Ante"); err != nil {
		t.Fatalf("Dig: %v", err)
	}
	if z := g.Card(top).Zone; z != engine.Ante {
		t.Fatalf("zone = %v, want Ante", z)
	}
	if _, err := resolveNow(t, g, p, c, nil, "DB$ ChangeZoneAll | ChangeType$ Card | Origin$ Ante | Destination$ Graveyard"); err != nil {
		t.Fatalf("ChangeZoneAll: %v", err)
	}
	if z := g.Card(top).Zone; z != engine.Graveyard {
		t.Errorf("zone = %v, want Graveyard", z)
	}
}

// FaceDown$ on a known-origin move to the battlefield (Shorecrasher
// Elemental, The Cyber-Controller): the card turns face down before it
// moves, ChangeZoneEffect.java:670-672, with FaceDownPower$/Toughness$ set;
// a hidden Origin$ stays refused.
func TestChangeZoneFaceDownEntersFaceDown(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGame(t)
	exiled := g.NewCard(creatureDefPT(t, "5", "5"), p, engine.Exile)
	c := engine.NewScriptedController()
	if _, err := resolveNow(t, g, p, c, []engine.EntityID{engine.CardEntity(exiled)},
		"DB$ ChangeZone | ValidTgts$ Card | Origin$ Exile | Destination$ Battlefield | FaceDown$ True | FaceDownPower$ 3 | FaceDownToughness$ 3"); err != nil {
		t.Fatalf("ChangeZone: %v", err)
	}
	card := g.Card(exiled)
	if card.Zone != engine.Battlefield || !card.IsFaceDown() {
		t.Fatalf("zone = %v face down = %v, want Battlefield, face down", card.Zone, card.IsFaceDown())
	}
	if pw, _ := card.Power(); pw != 3 {
		t.Errorf("face-down power = %d, want 3", pw)
	}
	g.NewCard(creatureDefPT(t, "1", "1"), p, engine.Library)
	if _, err := resolveNow(t, g, p, c, nil,
		"DB$ ChangeZone | Origin$ Library | Destination$ Exile | Hidden$ True | FaceDown$ True"); err == nil {
		t.Error("hidden FaceDown$ resolved, want a refusal")
	}
}

// Darkpact: GainOwnership names TgtZone$ Ante at targeting only, then
// ExchangeZone swaps the ante card, now owned by the caster, with the top card
// of their library (OwnershipGainEffect.java:13-22, ZoneExchangeEffect.java:
// 35-103).
func TestGainOwnershipOfAnteCardThenExchangeWithLibraryTop(t *testing.T) {
	t.Parallel()

	g, p, other := newTwoPlayerGame(t)
	anted := g.NewCard(creatureDefPT(t, "3", "3"), other, engine.Ante)
	top := g.NewCard(creatureDefPT(t, "2", "2"), p, engine.Library)
	c := engine.NewScriptedController()
	c.QueueCardChoice([]engine.CardID{top})
	if _, err := resolveNow(t, g, p, c, []engine.EntityID{engine.CardEntity(anted)},
		"DB$ GainOwnership | ValidTgts$ Card | TgtZone$ Ante | SubAbility$ ExchangeAnte",
		"ExchangeAnte", "DB$ ExchangeZone | Zone1$ Ante | Zone2$ Library | Object$ ParentTarget | ValidExchange$ Card.TopLibrary | Mandatory$ True"); err != nil {
		t.Fatalf("Darkpact chain: %v", err)
	}
	if got := g.Card(anted); got.Owner != p || got.Zone != engine.Library {
		t.Errorf("ante card: owner %v zone %v, want the caster's, in the library", got.Owner, got.Zone)
	}
	if z := g.Card(top).Zone; z != engine.Ante {
		t.Errorf("library top zone = %v, want Ante", z)
	}
}
