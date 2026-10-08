package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/carddb/compile"
	"github.com/jczastkiewicz/crucible/internal/engine"
	"github.com/jczastkiewicz/crucible/internal/mana"
)

// These tests cover ControlOpponentsSearchingLibrary$ (Opposition Agent,
// ADR-0040): while an affected player searches their library, the static's
// controller is the one making the decision, visible through
// Game.ControllingPlayer inside the choice, and the grant ends with it.

// searchRecorder wraps a ScriptedController and records who controls the
// searching player at the moment the search choice is asked.
type searchRecorder struct {
	*engine.ScriptedController
	seen []engine.PlayerID
}

func (r *searchRecorder) ChooseCardsForEffect(g *engine.Game, decider engine.PlayerID, source engine.CardID, options []engine.CardID, lo, hi int) []engine.CardID {
	r.seen = append(r.seen, g.ControllingPlayer(decider))
	return r.ScriptedController.ChooseCardsForEffect(g, decider, source, options, lo, hi)
}

func oppositionAgentLikeDef(t *testing.T) *compile.Card {
	t.Helper()
	return scriptDef(t, "Test Opposition Agent", "Creature Human",
		"S:Mode$ Continuous | Affected$ Opponent | ControlOpponentsSearchingLibrary$ You | Description$ x")
}

// search casts a library tutor for pid and resolves it under rec.
func search(t *testing.T, g *engine.Game, pid engine.PlayerID, origin string, rec *searchRecorder, want engine.CardID) {
	t.Helper()
	rec.QueueCardChoice([]engine.CardID{want})
	def := etbChainDef(t, "Test Tutor "+origin, "DB$ ChangeZone | Origin$ "+origin+" | Destination$ Hand | ChangeType$ Creature")
	g.Player(pid).ManaPool.Add(mana.Green, 1)
	card := g.NewCard(def, pid, engine.Hand)
	if !g.CastSpell(pid, card, rec) {
		t.Fatal("CastSpell failed")
	}
	if err := g.ResolveStack(engine.NewRegistry(), rec); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
}

func TestSearchControlHandsAnOpponentsSearchToTheStaticsController(t *testing.T) {
	t.Parallel()

	g, p, other, _ := newThreePlayerGame(t)
	g.NewCard(oppositionAgentLikeDef(t), p, engine.Battlefield)
	engine.CheckStateBasedActions(g, engine.NewScriptedController())
	wanted := g.NewCard(creatureDefPT(t, "2", "2"), other, engine.Library)

	rec := &searchRecorder{ScriptedController: engine.NewScriptedController()}
	g.SetTurnState(1, other, engine.Main1)
	search(t, g, other, "Library", rec, wanted)

	if len(rec.seen) != 1 || rec.seen[0] != p {
		t.Errorf("controller of the searcher during the search = %v, want [%v]", rec.seen, p)
	}
	if got := g.ControllingPlayer(other); got != engine.NoPlayer {
		t.Errorf("controller after the search = %v, want none", got)
	}
	if z := g.Card(wanted).Zone; z != engine.Hand {
		t.Errorf("found card zone = %v, want Hand", z)
	}
}

func TestSearchControlLeavesOtherSearchesAlone(t *testing.T) {
	t.Parallel()

	g, p, other, _ := newThreePlayerGame(t)
	g.NewCard(oppositionAgentLikeDef(t), p, engine.Battlefield)
	engine.CheckStateBasedActions(g, engine.NewScriptedController())

	// The Agent's own controller is not "an opponent".
	own := g.NewCard(creatureDefPT(t, "2", "2"), p, engine.Library)
	rec := &searchRecorder{ScriptedController: engine.NewScriptedController()}
	search(t, g, p, "Library", rec, own)
	if len(rec.seen) != 1 || rec.seen[0] != engine.NoPlayer {
		t.Errorf("own search controller = %v, want [none]", rec.seen)
	}

	// A hand pick is a hidden-origin choice but not a library search.
	held := g.NewCard(creatureDefPT(t, "2", "2"), other, engine.Hand)
	g.SetTurnState(1, other, engine.Main1)
	rec = &searchRecorder{ScriptedController: engine.NewScriptedController()}
	search(t, g, other, "Hand", rec, held)
	if len(rec.seen) != 1 || rec.seen[0] != engine.NoPlayer {
		t.Errorf("hand pick controller = %v, want [none]", rec.seen)
	}
}

// TestSearchControlYieldsToANewerMindslaverGrant proves the grant is keyed by
// the static's timestamp: Mindslaver's later grant stays the newest, so its
// controller still decides during the search (Java's TreeMap.lastEntry).
func TestSearchControlYieldsToANewerMindslaverGrant(t *testing.T) {
	t.Parallel()

	g, p, other, third := newThreePlayerGame(t)
	g.NewCard(oppositionAgentLikeDef(t), p, engine.Battlefield)
	c := engine.NewScriptedController()
	if _, err := resolveNow(t, g, third, c, []engine.EntityID{engine.PlayerEntity(other)}, "DB$ ControlPlayer | ValidTgts$ Player"); err != nil {
		t.Fatal(err)
	}
	advanceToPhase(t, g, c, 2, engine.Main1)
	wanted := g.NewCard(creatureDefPT(t, "2", "2"), other, engine.Library)

	rec := &searchRecorder{ScriptedController: c}
	search(t, g, other, "Library", rec, wanted)
	if len(rec.seen) != 1 || rec.seen[0] != third {
		t.Errorf("controller during the search = %v, want [%v]", rec.seen, third)
	}
	wantControl(t, g, other, third) // the Mindslaver grant outlives the search
}
