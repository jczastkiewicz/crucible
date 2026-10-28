package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
	"github.com/jczastkiewicz/crucible/internal/mana"
)

// specializeAlora puts Alora in zone, pushes a Mode$ Specialize SetState aimed at
// her from a host that chose color, and resolves it.
func specializeAlora(t *testing.T, g *engine.Game, p engine.PlayerID, zone engine.ZoneType, color mana.Colors) engine.CardID {
	t.Helper()
	alora := g.NewCard(corpusCard(t, "Alora, Rogue Companion"), p, zone)
	host := pushPlayLine(t, g, p, []engine.EntityID{engine.CardEntity(alora)}, "DB$ SetState | Mode$ Specialize | Defined$ Targeted")
	g.Card(host).Memory.SetChosenColors(color)
	if err := g.ResolveStack(engine.NewRegistry(), engine.NewScriptedController()); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	return alora
}

// Mode$ Specialize (ADR-0050): the card takes the face of the chosen colour,
// in the hand as well, keeps it through a zone change, and a second
// Specialize does nothing (Card.canSpecialize).
func TestSpecializeTakesTheChosenFaceAndKeepsItAcrossZones(t *testing.T) {
	t.Parallel()
	g, p, _ := newTwoPlayerGameOn(t, scenarioDB(t))
	alora := specializeAlora(t, g, p, engine.Hand, mana.Blue)
	if got := g.Card(alora).Name(); got != "Alora, Cheerful Thief" {
		t.Fatalf("specialized name = %q, want the blue face", got)
	}
	g.Move(alora, engine.Graveyard, p)
	if got := g.Card(alora).Name(); got != "Alora, Cheerful Thief" {
		t.Errorf("name in the graveyard = %q, the face did not stay", got)
	}
	host := pushPlayLine(t, g, p, []engine.EntityID{engine.CardEntity(alora)}, "DB$ SetState | Mode$ Specialize | Defined$ Targeted")
	g.Card(host).Memory.SetChosenColors(mana.Red)
	if err := g.ResolveStack(engine.NewRegistry(), engine.NewScriptedController()); err != nil {
		t.Fatalf("second Specialize: %v", err)
	}
	if got := g.Card(alora).Name(); got != "Alora, Cheerful Thief" {
		t.Errorf("second Specialize changed the name to %q", got)
	}
}

// Without exactly one chosen colour the effect is an error, not a guess.
func TestSpecializeWithoutAChosenColorIsAnError(t *testing.T) {
	t.Parallel()
	g, p, _ := newTwoPlayerGameOn(t, scenarioDB(t))
	alora := g.NewCard(corpusCard(t, "Alora, Rogue Companion"), p, engine.Hand)
	pushPlayLine(t, g, p, []engine.EntityID{engine.CardEntity(alora)}, "DB$ SetState | Mode$ Specialize | Defined$ Targeted")
	if err := g.ResolveStack(engine.NewRegistry(), engine.NewScriptedController()); err == nil {
		t.Fatal("Specialize with no chosen colour resolved")
	}
}

// Mode$ Specializes fires for the card that specialized (TriggerSpecializes,
// ValidCard$ only).
func TestSpecializesTriggerFiresForTheSpecializedCard(t *testing.T) {
	t.Parallel()
	g, p, _ := newTwoPlayerGameOn(t, scenarioDB(t))
	g.NewCard(triggerWatcherDef(t, "Watcher", "Mode$ Specializes | ValidCard$ Card | TriggerZones$ Battlefield | Execute$ TrigGain", "TrigGain", "DB$ GainLife | Defined$ You | LifeAmount$ 2"), p, engine.Battlefield)
	before := g.Player(p).Life
	specializeAlora(t, g, p, engine.Hand, mana.White)
	if got := g.Player(p).Life - before; got != 2 {
		t.Errorf("life gained = %d, want 2 from the Specializes trigger", got)
	}
}
