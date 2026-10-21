package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
)

// TestProtectionAllValidPlayersGrantsThePlayerProtection is Faith's Shield's
// second half: ProtectAllEffect gives each ValidPlayers$ player "Protection
// from <color>", which ends with the turn.
func TestProtectionAllValidPlayersGrantsThePlayerProtection(t *testing.T) {
	t.Parallel()

	g, p, other := newTwoPlayerGame(t)
	c := engine.NewScriptedController()
	if _, err := resolveNow(t, g, p, c, nil,
		"DB$ ProtectionAll | ValidCards$ Permanent.YouCtrl | ValidPlayers$ You | Gains$ red"); err != nil {
		t.Fatal(err)
	}
	engine.CheckStateBasedActions(g, c)
	if !g.Player(p).HasKeyword("Protection") || g.Player(other).HasKeyword("Protection") {
		t.Fatalf("protection: you %v, opponent %v, want true false", g.Player(p).HasKeyword("Protection"), g.Player(other).HasKeyword("Protection"))
	}
	advanceToPhase(t, g, c, 2, engine.Main1)
	engine.CheckStateBasedActions(g, c)
	if g.Player(p).HasKeyword("Protection") {
		t.Error("the player protection outlived the turn")
	}
}

// ValidPlayers$ alone, with Duration$ Permanent, grants for good.
func TestProtectionAllValidPlayersAlonePermanent(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGame(t)
	c := engine.NewScriptedController()
	if _, err := resolveNow(t, g, p, c, nil,
		"DB$ ProtectionAll | ValidPlayers$ You | Gains$ red | Duration$ Permanent"); err != nil {
		t.Fatal(err)
	}
	advanceToPhase(t, g, c, 2, engine.Main1)
	engine.CheckStateBasedActions(g, c)
	if !g.Player(p).HasKeyword("Protection") {
		t.Error("a Permanent player protection ended with the turn")
	}
}

func TestProtectionAllNeedsCardsOrPlayersAndAReadablePlayer(t *testing.T) {
	t.Parallel()

	for _, line := range []string{
		"DB$ ProtectionAll | Gains$ red",
		"DB$ ProtectionAll | ValidPlayers$ Bogus | Gains$ red",
	} {
		g, p, _ := newTwoPlayerGame(t)
		if _, err := resolveNow(t, g, p, engine.NewScriptedController(), nil, line); err == nil {
			t.Errorf("%q resolved, want an error", line)
		}
	}
}
