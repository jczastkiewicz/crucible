package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
)

// PumpAll's SharedKeywordsZone$ grants only the keywords some card in those
// zones that SharedRestrictions$ accepts has (Concerted Effort,
// PumpAllEffect.java:151-157): flying is on the controller's board, trample
// only on the opponent's.
func TestPumpAllSharedKeywordsGrantsWhatTheZoneHas(t *testing.T) {
	t.Parallel()

	g, p, other := newTwoPlayerGame(t)
	flier := g.NewCard(creatureDefPTKeywords(t, "1", "1", "Flying"), p, engine.Battlefield)
	plain := g.NewCard(creatureDefPT(t, "2", "2"), p, engine.Battlefield)
	g.NewCard(creatureDefPTKeywords(t, "1", "1", "Trample"), other, engine.Battlefield)
	if _, err := resolveNow(t, g, p, engine.NewScriptedController(), nil,
		"DB$ PumpAll | ValidCards$ Creature.YouCtrl | KW$ Flying & Trample | SharedKeywordsZone$ Battlefield | SharedRestrictions$ Creature.YouCtrl"); err != nil {
		t.Fatal(err)
	}
	if !g.Card(plain).HasKeyword("Flying") || !g.Card(flier).HasKeyword("Flying") {
		t.Error("a creature lacks flying, want both: a creature you control has it")
	}
	if g.Card(plain).HasKeyword("Trample") {
		t.Error("the plain creature gained trample, want none: only the opponent's creature has it")
	}
}

// Pump with SharedKeywordsZone$ on a restriction naming a remembered or
// triggering card is refused, not guessed.
func TestPumpSharedKeywordsRefusesARememberedRestriction(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGame(t)
	if _, err := resolveNow(t, g, p, engine.NewScriptedController(), nil,
		"DB$ Pump | Defined$ Self | KW$ Flying | SharedKeywordsZone$ Library | SharedRestrictions$ Card.IsRemembered"); err == nil {
		t.Fatal("registered, want an error")
	}
}

// A SharedKeywordsZone$ naming no zone is refused.
func TestPumpSharedKeywordsRefusesAnUnknownZone(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGame(t)
	if _, err := resolveNow(t, g, p, engine.NewScriptedController(), nil,
		"DB$ Pump | Defined$ Self | KW$ Flying | SharedKeywordsZone$ Nowhere"); err == nil {
		t.Fatal("registered, want an error")
	}
}
