package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
)

// These tests cover the Storied keyword and Condition$ EnduringStory (Fili
// the Pathfinder, Thorin Oakenshield): a permanent with Storied gives its
// controller an enduring story, for the rest of the game, once they control
// three or more historic permanents.

func TestEnduringStoryIsGainedAtThreeHistoricPermanentsAndKept(t *testing.T) {
	t.Parallel()

	g, p, other := newTwoPlayerGame(t)
	sc := engine.NewScriptedController()
	fili := g.NewCard(copyTestDef(t, "Test Fili", "Legendary Creature Elf", "2", "2", "K:Storied",
		"S:Mode$ Continuous | Affected$ Creature.YouCtrl | AddPower$ 1 | AddToughness$ 1 | Condition$ EnduringStory | Description$ x"),
		p, engine.Battlefield)
	bear := g.NewCard(copyTestDef(t, "Test Bear", "Creature Elf", "2", "2"), p, engine.Battlefield)
	// The opponent has a Storied permanent and no history: no story for them.
	theirs := g.NewCard(copyTestDef(t, "Their Fili", "Creature Elf", "2", "2", "K:Storied",
		"S:Mode$ Continuous | Affected$ Creature.YouCtrl | AddPower$ 1 | AddToughness$ 1 | Condition$ EnduringStory | Description$ x"),
		other, engine.Battlefield)

	engine.CheckStateBasedActions(g, sc)
	wantPT(t, g, bear, 2, 2)
	if g.Player(p).EnduringStory {
		t.Fatal("an enduring story with one historic permanent")
	}

	// Two artifacts make three historic permanents with the legendary Fili.
	a1 := g.NewCard(copyTestDef(t, "Test Rod", "Artifact", "", ""), p, engine.Battlefield)
	g.NewCard(copyTestDef(t, "Test Rod", "Artifact", "", ""), p, engine.Battlefield)
	engine.CheckStateBasedActions(g, sc)
	if !g.Player(p).EnduringStory {
		t.Fatal("no enduring story with three historic permanents")
	}
	wantPT(t, g, bear, 3, 3)
	wantPT(t, g, fili, 3, 3)
	if g.Player(other).EnduringStory {
		t.Error("the opponent has an enduring story")
	}
	wantPT(t, g, theirs, 2, 2)

	// For the rest of the game: losing a historic permanent loses nothing.
	g.Move(a1, engine.Graveyard, p)
	engine.CheckStateBasedActions(g, sc)
	wantPT(t, g, bear, 3, 3)
}

func TestEnduringStoryNeedsAStoriedPermanentOnTheBattlefield(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGame(t)
	sc := engine.NewScriptedController()
	g.NewCard(copyTestDef(t, "Test Fili", "Legendary Creature Elf", "2", "2", "K:Storied"), p, engine.Hand)
	g.NewCard(copyTestDef(t, "Test Rod", "Artifact", "", ""), p, engine.Battlefield)
	g.NewCard(copyTestDef(t, "Test Rod", "Artifact", "", ""), p, engine.Battlefield)
	g.NewCard(copyTestDef(t, "Test Rod", "Artifact", "", ""), p, engine.Battlefield)
	engine.CheckStateBasedActions(g, sc)
	if g.Player(p).EnduringStory {
		t.Error("an enduring story from a Storied card in hand")
	}
}
