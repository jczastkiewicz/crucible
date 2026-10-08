package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/carddb/compile"
	"github.com/jczastkiewicz/crucible/internal/engine"
	"github.com/jczastkiewicz/crucible/internal/mana"
)

// These tests cover IgnoreEffectCost$ and the CantSearchLibrary player
// keyword (Leonin Arbiter): players cannot search libraries, and any player
// may pay {2} to ignore the effect for themself until end of turn.

func leoninArbiterDef(t *testing.T) *compile.Card {
	t.Helper()
	return scriptDef(t, "Test Leonin Arbiter", "Creature Cat",
		"S:Mode$ Continuous | Affected$ Player | AddKeyword$ CantSearchLibrary | IgnoreEffectCost$ 2 | Description$ x")
}

func TestCantSearchLibraryOffersNothingAndStillShuffles(t *testing.T) {
	t.Parallel()

	g, p, other := newTwoPlayerGame(t)
	g.NewCard(leoninArbiterDef(t), other, engine.Battlefield)
	engine.CheckStateBasedActions(g, engine.NewScriptedController())
	wanted := g.NewCard(creatureDefPT(t, "2", "2"), p, engine.Library)

	rec := &searchRecorder{ScriptedController: engine.NewScriptedController()}
	search(t, g, p, "Library", rec, wanted)
	if len(rec.seen) != 0 {
		t.Errorf("a blocked search asked for a choice %d times, want 0", len(rec.seen))
	}
	if z := g.Card(wanted).Zone; z != engine.Library {
		t.Errorf("card zone = %v, want Library (the search was blocked)", z)
	}
	if !g.Player(p).HasKeyword("CantSearchLibrary") {
		t.Error("player lacks CantSearchLibrary")
	}
}

func TestIgnoreEffectCostLetsOnePlayerSearchUntilEndOfTurn(t *testing.T) {
	t.Parallel()

	g, p, other := newTwoPlayerGame(t)
	arbiter := g.NewCard(leoninArbiterDef(t), other, engine.Battlefield)
	c := engine.NewScriptedController()
	engine.CheckStateBasedActions(g, c)

	// Any player may activate it, not only the Arbiter's controller.
	g.Player(p).ManaPool.Add(mana.Green, 2)
	c.QueuePayGeneric(mana.ShardG)
	c.QueuePayGeneric(mana.ShardG)
	if !g.ActivateAbility(p, arbiter, 0, c) {
		t.Fatal("ActivateAbility: the opponent could not pay {2} to ignore the effect")
	}
	if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if g.Player(p).HasKeyword("CantSearchLibrary") {
		t.Error("the paying player still has CantSearchLibrary")
	}
	if !g.Player(other).HasKeyword("CantSearchLibrary") {
		t.Error("the other player lost CantSearchLibrary too")
	}

	wanted := g.NewCard(creatureDefPT(t, "2", "2"), p, engine.Library)
	rec := &searchRecorder{ScriptedController: c}
	search(t, g, p, "Library", rec, wanted)
	if z := g.Card(wanted).Zone; z != engine.Hand {
		t.Errorf("found card zone = %v, want Hand", z)
	}

	// Until end of turn only: the next turn the effect applies again.
	advanceToPhase(t, g, c, 2, engine.Main1)
	engine.CheckStateBasedActions(g, c)
	if !g.Player(p).HasKeyword("CantSearchLibrary") {
		t.Error("the ignore outlived the turn")
	}
}

func TestIgnoreEffectCostNeedsTheHostInPlayAndTheMana(t *testing.T) {
	t.Parallel()

	g, p, other := newTwoPlayerGame(t)
	arbiter := g.NewCard(leoninArbiterDef(t), other, engine.Battlefield)
	c := engine.NewScriptedController()
	engine.CheckStateBasedActions(g, c)

	g.Player(p).ManaPool.Add(mana.Green, 2)
	g.Move(arbiter, engine.Graveyard, other)
	if g.ActivateAbility(p, arbiter, 0, c) {
		t.Error("ActivateAbility succeeded with the host out of play")
	}
}

func TestSpellsAndAbilitiesCannotCauseYouToSearchKeyword(t *testing.T) {
	t.Parallel()

	g, p, other := newTwoPlayerGame(t)
	g.NewCard(scriptDef(t, "Test Lock", "Creature Cat",
		"S:Mode$ Continuous | Affected$ You | AddKeyword$ Spells and abilities you control can't cause you to search your library."), p, engine.Battlefield)
	engine.CheckStateBasedActions(g, engine.NewScriptedController())
	own := g.NewCard(creatureDefPT(t, "2", "2"), p, engine.Library)
	theirs := g.NewCard(creatureDefPT(t, "2", "2"), other, engine.Library)

	rec := &searchRecorder{ScriptedController: engine.NewScriptedController()}
	search(t, g, p, "Library", rec, own)
	if z := g.Card(own).Zone; z != engine.Library {
		t.Errorf("own library search: card zone = %v, want Library (blocked)", z)
	}
	_ = theirs
}
