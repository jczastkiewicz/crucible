package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
)

// Untap.doUntap (Untap.java:104-157): UntapAdjust caps how many permanents of
// a type untap and the player picks which; a Mode$ UntapOtherPlayer static
// untaps its ValidCard$ permanents in the other player's step too.

func tappedCreatures(t *testing.T, g *engine.Game, p engine.PlayerID, n int) []engine.CardID {
	t.Helper()
	ids := make([]engine.CardID, n)
	for i := range ids {
		ids[i] = g.NewCard(creatureDefPT(t, "1", "1"), p, engine.Battlefield)
		g.Card(ids[i]).Tapped = true
	}
	return ids
}

func TestUntapAdjustLetsPlayerPickWhichCreatureUntaps(t *testing.T) {
	t.Parallel()

	g, p, other := newTwoPlayerGame(t)
	g.NewCard(scriptDef(t, "Test Smoke", "Enchantment",
		"S:Mode$ Continuous | Affected$ Player | AddKeyword$ UntapAdjust:Creature:1"), other, engine.Battlefield)
	creatures := tappedCreatures(t, g, p, 3)
	engine.CheckStateBasedActions(g, engine.NewScriptedController())
	c := engine.NewScriptedController()
	c.QueueCardChoice([]engine.CardID{creatures[1]})

	g.StartTurn(p, c)

	for i, id := range creatures {
		if got, want := g.Card(id).Tapped, i != 1; got != want {
			t.Errorf("creature %d tapped = %v, want %v", i, got, want)
		}
	}
}

func TestUntapOtherPlayerUntapsInAnotherPlayersStep(t *testing.T) {
	t.Parallel()

	g, p, other := newTwoPlayerGame(t)
	g.NewCard(scriptDef(t, "Test Seedborn", "Enchantment",
		"S:Mode$ UntapOtherPlayer | ValidCard$ Creature.YouCtrl"), other, engine.Battlefield)
	mine := tappedCreatures(t, g, p, 1)
	theirs := tappedCreatures(t, g, other, 1)

	g.StartTurn(p, engine.NewScriptedController())

	if g.Card(theirs[0]).Tapped {
		t.Error("the static's creature stayed tapped in the other player's untap step")
	}
	if g.Card(mine[0]).Tapped {
		t.Error("the active player's own creature stayed tapped")
	}
}

func TestOnlyUntapChosenUntapsOnlyThePickedType(t *testing.T) {
	t.Parallel()

	g, p, other := newTwoPlayerGame(t)
	g.NewCard(scriptDef(t, "Test Winter Orb", "Artifact",
		"S:Mode$ Continuous | Affected$ Player | AddKeyword$ OnlyUntapChosen:Artifact,Creature,Land"), other, engine.Battlefield)
	creature := tappedCreatures(t, g, p, 1)[0]
	rock := g.NewCard(scriptDef(t, "Test Rock", "Artifact"), p, engine.Battlefield)
	g.Card(rock).Tapped = true
	engine.CheckStateBasedActions(g, engine.NewScriptedController())
	c := engine.NewScriptedController()
	c.QueueOptionNamed("Creature")

	g.StartTurn(p, c)

	if g.Card(creature).Tapped || !g.Card(rock).Tapped {
		t.Errorf("creature tapped = %v, artifact tapped = %v; want only the creature untapped", g.Card(creature).Tapped, g.Card(rock).Tapped)
	}
}
