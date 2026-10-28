package engine_test

import (
	"slices"
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
)

// These tests cover the LimitSearchLibrary player keyword (Aven Mindcensor):
// a decider under it searches the top four cards of the library only
// (ChangeZoneEffect.java:1023-1031).

const avenLikeLine = "S:Mode$ Continuous | Affected$ Player.Opponent | AddKeyword$ LimitSearchLibrary | Description$ x"

func TestLimitSearchLibraryOffersTheTopFourCardsOnly(t *testing.T) {
	t.Parallel()

	g, p, other := newTwoPlayerGame(t)
	g.NewCard(scriptDef(t, "Test Aven Mindcensor", "Creature Bird", avenLikeLine), other, engine.Battlefield)
	engine.CheckStateBasedActions(g, engine.NewScriptedController())
	lib := libraryCards(t, g, p, 6)

	rec := &offerRecorder{ScriptedController: engine.NewScriptedController()}
	rec.QueueCardChoice(lib[3:4])
	resolveRecorded(t, g, p, rec, "DB$ ChangeZone | Origin$ Library | Destination$ Hand | ChangeType$ Card | Mandatory$ True")

	if len(rec.offers) != 1 || !slices.Equal(rec.offers[0], lib[:4]) {
		t.Errorf("offers = %v, want the top four %v", rec.offers, lib[:4])
	}
	if z := g.Card(lib[3]).Zone; z != engine.Hand {
		t.Errorf("fourth card zone = %v, want Hand", z)
	}
}

func TestLimitSearchLibraryWithAnEmptyLibraryIsStillASearch(t *testing.T) {
	t.Parallel()

	g, p, other := newTwoPlayerGame(t)
	g.NewCard(scriptDef(t, "Test Aven Mindcensor", "Creature Bird", avenLikeLine), other, engine.Battlefield)
	engine.CheckStateBasedActions(g, engine.NewScriptedController())

	rec := &offerRecorder{ScriptedController: engine.NewScriptedController()}
	resolveRecorded(t, g, p, rec, "DB$ ChangeZone | Origin$ Library | Destination$ Hand | ChangeType$ Card")

	if got := measure(t, g, p, "Count$SearchedLibrary.You"); got != 1 {
		t.Errorf("searches counted = %d, want 1: getSearchableLibrary returns an empty list, not null", got)
	}
}

// resolveRecorded resolves the ChangeZone line trig as an ability of a host on
// p's battlefield, answering every decision through rec.
func resolveRecorded(t *testing.T, g *engine.Game, p engine.PlayerID, rec *offerRecorder, trig string) {
	t.Helper()
	def := etbChainDef(t, "Test Now", trig)
	host := g.NewCard(def, p, engine.Battlefield)
	sub := def.Faces[0].Triggers[0].Subs[0].Ability
	g.PushAbility(engine.Ability{API: engine.APIChangeZone, Source: host, Controller: p, Params: sub})
	if err := g.ResolveStack(engine.NewRegistry(), rec); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
}
