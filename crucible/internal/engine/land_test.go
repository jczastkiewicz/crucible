package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
)

// Playing a land moves it straight to the battlefield -- no cost, no stack
// -- and counts against the turn's own land-play limit.
func TestPlayLandMovesCardToBattlefieldAndCountsIt(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a")
	p := g.Players()[0]
	g.SetTurnState(1, p, engine.Main1)
	plains := g.NewCard(landDef(t, "Plains", "Basic Land Plains"), p, engine.Hand)

	if !g.PlayLand(p, plains) {
		t.Fatal("PlayLand failed on a plain land in hand during the active player's Main1")
	}
	if g.Card(plains).Zone != engine.Battlefield {
		t.Errorf("land zone = %v, want Battlefield", g.Card(plains).Zone)
	}
	if got := g.Player(p).LandsPlayed; got != 1 {
		t.Errorf("LandsPlayed = %d, want 1", got)
	}
}

// CR 305.2: one land per turn. A second land the same turn is declined, not
// an error -- the card stays in hand.
func TestPlayLandFailsAfterTheTurnsLimitIsSpent(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a")
	p := g.Players()[0]
	g.SetTurnState(1, p, engine.Main1)
	first := g.NewCard(landDef(t, "Plains", "Basic Land Plains"), p, engine.Hand)
	second := g.NewCard(landDef(t, "Island", "Basic Land Island"), p, engine.Hand)

	if !g.PlayLand(p, first) {
		t.Fatal("PlayLand failed on the first land this turn")
	}
	if g.PlayLand(p, second) {
		t.Fatal("PlayLand succeeded on a second land the same turn")
	}
	if g.Card(second).Zone != engine.Hand {
		t.Error("second land left hand despite the declined play")
	}
}

// CR 305.3: only the active player may play a land.
func TestPlayLandFailsWhenNotActivePlayer(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	active, other := g.Players()[0], g.Players()[1]
	g.SetTurnState(1, active, engine.Main1)
	land := g.NewCard(landDef(t, "Plains", "Basic Land Plains"), other, engine.Hand)

	if g.PlayLand(other, land) {
		t.Fatal("PlayLand succeeded for a player who is not the active player")
	}
}

// CR 305.3: a land is played at sorcery speed -- one of the active player's
// own main phases, not during combat.
func TestPlayLandFailsOutsideAMainPhase(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a")
	p := g.Players()[0]
	g.SetTurnState(1, p, engine.CombatDamage)
	land := g.NewCard(landDef(t, "Plains", "Basic Land Plains"), p, engine.Hand)

	if g.PlayLand(p, land) {
		t.Fatal("PlayLand succeeded outside a main phase")
	}
}

// CR 305.3 collapses to "the stack is empty" in this port, since nothing can
// respond to what is already there yet.
func TestPlayLandFailsWhenStackIsNotEmpty(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a")
	p := g.Players()[0]
	g.SetTurnState(1, p, engine.Main1)
	land := g.NewCard(landDef(t, "Plains", "Basic Land Plains"), p, engine.Hand)
	g.PushAbility(engine.Ability{})

	if g.PlayLand(p, land) {
		t.Fatal("PlayLand succeeded with something already on the stack")
	}
}

// A land already on the battlefield has nothing left to play.
func TestPlayLandFailsWhenCardIsNotInHand(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a")
	p := g.Players()[0]
	g.SetTurnState(1, p, engine.Main1)
	land := g.NewCard(landDef(t, "Plains", "Basic Land Plains"), p, engine.Battlefield)

	if g.PlayLand(p, land) {
		t.Fatal("PlayLand succeeded on a card already on the battlefield")
	}
}

// A nonland card in hand is not playable through PlayLand.
func TestPlayLandFailsWhenCardIsNotALand(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a")
	p := g.Players()[0]
	g.SetTurnState(1, p, engine.Main1)
	bear := g.NewCard(creatureDefPT(t, "2", "2"), p, engine.Hand)

	if g.PlayLand(p, bear) {
		t.Fatal("PlayLand succeeded on a creature card")
	}
}

// cleanupStep (turn.go) rolls LandsPlayed into LandsPlayedLastTurn and
// resets it to zero for every player, not just the active one -- the same
// "every player, not just the active one" scope emptyManaPools already has
// for CR 500.4.
func TestCleanupResetsLandsPlayedForEveryPlayer(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	active, other := g.Players()[0], g.Players()[1]
	g.Player(active).LandsPlayed = 1
	g.Player(other).LandsPlayed = 1
	g.SetTurnState(1, active, engine.EndOfTurn)

	g.AdvancePhase(engine.NewScriptedController())

	if got := g.Player(active).LandsPlayed; got != 0 {
		t.Errorf("active player's LandsPlayed after cleanup = %d, want 0", got)
	}
	if got := g.Player(active).LandsPlayedLastTurn; got != 1 {
		t.Errorf("active player's LandsPlayedLastTurn after cleanup = %d, want 1", got)
	}
	if got := g.Player(other).LandsPlayed; got != 0 {
		t.Errorf("other player's LandsPlayed after cleanup = %d, want 0", got)
	}
	if got := g.Player(other).LandsPlayedLastTurn; got != 1 {
		t.Errorf("other player's LandsPlayedLastTurn after cleanup = %d, want 1", got)
	}
}
