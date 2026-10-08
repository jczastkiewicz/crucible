package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
)

// These tests cover Pump KW$ on a player (PumpEffect.java:498-504, Defined$
// You): the keyword goes on the player until end of turn, or until the
// activator's next turn with Duration$ UntilYourNextTurn (Eon Frolicker,
// Noble Heritage), and DefinedKW$ fills in the protection-from-a-player form.

func TestPumpKeywordOnAPlayerLastsUntilEndOfTurn(t *testing.T) {
	t.Parallel()

	g, p, other := newTwoPlayerGame(t)
	c := engine.NewScriptedController()
	if _, err := resolveNow(t, g, p, c, nil, "DB$ Pump | Defined$ You | KW$ Hexproof"); err != nil {
		t.Fatal(err)
	}
	engine.CheckStateBasedActions(g, c)
	if !g.Player(p).HasKeyword("Hexproof") || g.Player(other).HasKeyword("Hexproof") {
		t.Fatalf("hexproof: you %v, opponent %v, want true false", g.Player(p).HasKeyword("Hexproof"), g.Player(other).HasKeyword("Hexproof"))
	}
	advanceToPhase(t, g, c, 2, engine.Main1)
	engine.CheckStateBasedActions(g, c)
	if g.Player(p).HasKeyword("Hexproof") {
		t.Error("the player keyword outlived the turn")
	}
}

func TestPumpKeywordOnAPlayerUntilYourNextTurnWithProtectionFromAPlayer(t *testing.T) {
	t.Parallel()

	g, p, other := newTwoPlayerGame(t)
	c := engine.NewScriptedController()
	if _, err := resolveNow(t, g, p, c, nil,
		"DB$ Pump | Defined$ You | KW$ Protection:Player.PlayerUID_ChosenPlayerUID:ChosenPlayerName | DefinedKW$ Opponent | Duration$ UntilYourNextTurn"); err != nil {
		t.Fatal(err)
	}
	engine.CheckStateBasedActions(g, c)
	if !g.Player(p).HasKeyword("Protection") {
		t.Fatal("you lack protection after the pump")
	}
	advanceToPhase(t, g, c, 2, engine.Main1)
	engine.CheckStateBasedActions(g, c)
	if !g.Player(p).HasKeyword("Protection") {
		t.Error("protection ended with the turn it was granted in")
	}
	advanceToPhase(t, g, c, 3, engine.Upkeep)
	engine.CheckStateBasedActions(g, c)
	if g.Player(p).HasKeyword("Protection") {
		t.Error("protection outlived the start of your next turn")
	}
	_ = other
}

func TestPumpUntilYourNextTurnOnACard(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGame(t)
	c := engine.NewScriptedController()
	target := g.NewCard(creatureDefPT(t, "2", "2"), p, engine.Battlefield)
	if _, err := resolveNow(t, g, p, c, []engine.EntityID{engine.CardEntity(target)}, "DB$ Pump | ValidTgts$ Creature | NumAtt$ 2 | Duration$ UntilYourNextTurn"); err != nil {
		t.Fatal(err)
	}
	engine.CheckStateBasedActions(g, c)
	advanceToPhase(t, g, c, 2, engine.Main1)
	engine.CheckStateBasedActions(g, c)
	if got, _ := g.Card(target).Power(); got != 4 {
		t.Errorf("power during the opponent's turn = %d, want 4", got)
	}
	advanceToPhase(t, g, c, 3, engine.Upkeep)
	engine.CheckStateBasedActions(g, c)
	if got, _ := g.Card(target).Power(); got != 2 {
		t.Errorf("power after your next turn began = %d, want 2", got)
	}
}
