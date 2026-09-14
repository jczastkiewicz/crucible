package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
)

// StartTurn announces the turn beginning before the phase it starts in --
// a recorder reading TurnBegan before the first PhaseBegan should not have
// to infer that turn 1 opens on Untap from context.
func TestStartTurnEmitsTurnBeganThenPhaseBegan(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	a, b := g.Players()[0], g.Players()[1]
	g.Player(a).Life, g.Player(b).Life = 20, 20
	var sink recordingSink
	g.SetSink(&sink)

	g.StartTurn(a, engine.NewScriptedController())

	if len(sink.events) < 2 {
		t.Fatalf("sink saw %d events, want at least 2", len(sink.events))
	}
	if sink.events[0].Kind != engine.TurnBegan || sink.events[0].Turn != 1 || sink.events[0].Active != a {
		t.Errorf("first event %+v, want TurnBegan turn=1 active=%v", sink.events[0], a)
	}
	if sink.events[1].Kind != engine.PhaseBegan || sink.events[1].Phase != engine.Untap {
		t.Errorf("second event %+v, want PhaseBegan phase=Untap", sink.events[1])
	}
}

// AdvancePhase emits PhaseBegan every step, and TurnBegan only on the one
// that wraps back to Untap -- a recorder should be able to count turns from
// TurnBegan alone, without also counting Cleanup-to-Untap transitions.
func TestAdvancePhaseEmitsPhaseBeganEveryStepAndTurnBeganOnlyOnWrap(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	a, b := g.Players()[0], g.Players()[1]
	g.Player(a).Life, g.Player(b).Life = 20, 20
	g.SetTurnState(1, a, engine.Cleanup)
	var sink recordingSink
	g.SetSink(&sink)

	g.AdvancePhase(engine.NewScriptedController()) // wraps to turn 2, Untap

	turnBegans, phaseBegans := 0, 0
	for _, e := range sink.events {
		switch e.Kind {
		case engine.TurnBegan:
			turnBegans++
			if e.Turn != 2 || e.Active != b {
				t.Errorf("TurnBegan %+v, want turn=2 active=%v", e, b)
			}
		case engine.PhaseBegan:
			phaseBegans++
			if e.Phase != engine.Untap {
				t.Errorf("PhaseBegan %+v, want phase=Untap", e)
			}
		}
	}
	if turnBegans != 1 {
		t.Errorf("saw %d TurnBegan events, want 1", turnBegans)
	}
	if phaseBegans != 1 {
		t.Errorf("saw %d PhaseBegan events, want 1", phaseBegans)
	}
	// TurnBegan has to come first: it is emitted before ActivePhase changes
	// and beginPhase runs.
	if sink.events[0].Kind != engine.TurnBegan {
		t.Errorf("first event is %s, want TurnBegan", sink.events[0].Kind)
	}
}

// A draw emits both events Move and drawStep are each responsible for, in
// the order they actually happened: the zone change first, then the
// draw-specific signal on top of it.
func TestDrawEmitsZoneChangedThenCardDrawn(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	a, b := g.Players()[0], g.Players()[1]
	g.Player(a).Life, g.Player(b).Life = 20, 20
	top := g.NewCard(nil, a, engine.Library)
	g.SetTurnState(2, a, engine.Upkeep)
	var sink recordingSink
	g.SetSink(&sink)

	g.AdvancePhase(engine.NewScriptedController()) // -> Draw

	var kinds []engine.EventKind
	for _, e := range sink.events {
		kinds = append(kinds, e.Kind)
	}
	zc, cd := -1, -1
	for i, k := range kinds {
		if k == engine.ZoneChanged {
			zc = i
		}
		if k == engine.CardDrawn {
			cd = i
		}
	}
	if zc == -1 || cd == -1 {
		t.Fatalf("events %v, want both ZoneChanged and CardDrawn", kinds)
	}
	if zc >= cd {
		t.Errorf("ZoneChanged at %d, CardDrawn at %d -- want ZoneChanged first", zc, cd)
	}
	if sink.events[cd].Source != top {
		t.Errorf("CardDrawn source %v, want %v", sink.events[cd].Source, top)
	}
}

func TestStartTurnEntersUntap(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	a := g.Players()[0]
	g.StartTurn(a, engine.NewScriptedController())

	if g.Turn() != 1 {
		t.Errorf("turn %d, want 1", g.Turn())
	}
	if g.ActivePlayer() != a {
		t.Errorf("active player %v, want %v", g.ActivePlayer(), a)
	}
	if g.ActivePhase() != engine.Untap {
		t.Errorf("phase %v, want Untap", g.ActivePhase())
	}
}

// The step order is PhaseType's declaration order, and AdvancePhase must
// walk every one of them, not skip silently to something interesting.
func TestAdvancePhaseWalksStepsInOrder(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	a, b := g.Players()[0], g.Players()[1]
	g.Player(a).Life, g.Player(b).Life = 20, 20
	g.StartTurn(a, engine.NewScriptedController())

	want := []engine.PhaseType{
		engine.Upkeep, engine.Draw, engine.Main1, engine.CombatBegin,
		engine.DeclareAttackers, engine.DeclareBlockers, engine.FirstStrikeDamage,
		engine.CombatDamage, engine.CombatEnd, engine.Main2, engine.EndOfTurn, engine.Cleanup,
	}
	for i, phase := range want {
		g.AdvancePhase(engine.NewScriptedController())
		if g.ActivePhase() != phase {
			t.Fatalf("step %d: phase %v, want %v", i, g.ActivePhase(), phase)
		}
		if g.Turn() != 1 {
			t.Fatalf("step %d: turn %d, want 1 (still turn 1 until Cleanup wraps)", i, g.Turn())
		}
	}
}

// Cleanup wraps to Untap of the next turn, and the active player rotates.
func TestAdvancePhaseWrapsToNextTurnAndRotatesPlayer(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	a, b := g.Players()[0], g.Players()[1]
	g.Player(a).Life, g.Player(b).Life = 20, 20
	g.StartTurn(a, engine.NewScriptedController())
	g.SetTurnState(1, a, engine.Cleanup)

	g.AdvancePhase(engine.NewScriptedController())

	if g.Turn() != 2 {
		t.Errorf("turn %d, want 2", g.Turn())
	}
	if g.ActivePlayer() != b {
		t.Errorf("active player %v, want %v", g.ActivePlayer(), b)
	}
	if g.ActivePhase() != engine.Untap {
		t.Errorf("phase %v, want Untap", g.ActivePhase())
	}
}

// A player who has lost is skipped in turn order -- play passes to the next
// one still in the game.
func TestAdvancePhaseSkipsLostPlayersInRotation(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b", "c")
	a, b, c := g.Players()[0], g.Players()[1], g.Players()[2]
	for _, id := range g.Players() {
		g.Player(id).Life = 20
	}
	g.Player(b).Lost = true
	g.SetTurnState(1, a, engine.Cleanup)

	g.AdvancePhase(engine.NewScriptedController())

	if g.ActivePlayer() != c {
		t.Errorf("active player %v, want %v (b has lost)", g.ActivePlayer(), c)
	}
}

// If everyone else has lost, turn order has nowhere to go -- this only
// happens when the game should already be over, so returning the same
// player rather than panicking is the safe fallback.
func TestAdvancePhaseWithNoPlayersLeftReturnsSamePlayer(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	a, b := g.Players()[0], g.Players()[1]
	g.Player(a).Life, g.Player(b).Life = 20, 20
	g.Player(b).Lost = true
	g.SetTurnState(1, a, engine.Cleanup)

	g.AdvancePhase(engine.NewScriptedController())

	if g.ActivePlayer() != a {
		t.Errorf("active player %v, want %v (the only player left)", g.ActivePlayer(), a)
	}
}

// If the active player has also lost -- a state that should not outlive the
// next state-based-action check, but nothing stops it being constructed --
// wrapping all the way around finds no one, and nextPlayerAfter's fallback
// returns the same player rather than an out-of-range index or a panic.
func TestAdvancePhaseWithEveryoneLostIncludingSelfReturnsSamePlayer(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	a, b := g.Players()[0], g.Players()[1]
	g.Player(a).Lost = true
	g.Player(b).Lost = true
	g.SetTurnState(1, a, engine.Cleanup)

	g.AdvancePhase(engine.NewScriptedController())

	if g.ActivePlayer() != a {
		t.Errorf("active player %v, want %v (nowhere left to pass to)", g.ActivePlayer(), a)
	}
}

func TestUntapClearsTappedAndSummonSickForTheActivePlayerOnly(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	a, b := g.Players()[0], g.Players()[1]
	g.Player(a).Life, g.Player(b).Life = 20, 20
	mine := g.NewCard(nil, a, engine.Battlefield)
	theirs := g.NewCard(nil, b, engine.Battlefield)
	g.Card(mine).Tapped, g.Card(mine).SummonSick = true, true
	g.Card(theirs).Tapped, g.Card(theirs).SummonSick = true, true

	g.StartTurn(a, engine.NewScriptedController())

	if c := g.Card(mine); c.Tapped || c.SummonSick {
		t.Errorf("active player's permanent tapped=%v summonsick=%v after untap, want false/false", c.Tapped, c.SummonSick)
	}
	if c := g.Card(theirs); !c.Tapped || !c.SummonSick {
		t.Errorf("opponent's permanent tapped=%v summonsick=%v after another player's untap, want true/true (untouched)", c.Tapped, c.SummonSick)
	}
}

// The top of the library is index 0: a fixture author's left-to-right order
// is top-to-bottom, and Load builds cards in that order.
func TestDrawMovesTopOfLibraryToHand(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	a, b := g.Players()[0], g.Players()[1]
	g.Player(a).Life, g.Player(b).Life = 20, 20
	top := g.NewCard(nil, a, engine.Library)
	rest := g.NewCard(nil, a, engine.Library)
	g.SetTurnState(2, a, engine.Upkeep) // past the turn-1 skip

	g.AdvancePhase(engine.NewScriptedController()) // -> Draw

	if g.ActivePhase() != engine.Draw {
		t.Fatalf("phase %v, want Draw", g.ActivePhase())
	}
	hand := g.Zone(engine.Hand, a).Cards()
	if len(hand) != 1 || hand[0] != top {
		t.Errorf("hand %v, want [%v]", hand, top)
	}
	if lib := g.Zone(engine.Library, a).Cards(); len(lib) != 1 || lib[0] != rest {
		t.Errorf("library %v, want [%v]", lib, rest)
	}
}

// CR 103.7a: the first player skips the draw step of their own first turn
// in a two-player game.
func TestDrawSkipsFirstPlayerFirstTurnTwoPlayer(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	a, b := g.Players()[0], g.Players()[1]
	g.Player(a).Life, g.Player(b).Life = 20, 20
	g.NewCard(nil, a, engine.Library)
	g.StartTurn(a, engine.NewScriptedController())

	g.AdvancePhase(engine.NewScriptedController()) // Upkeep
	g.AdvancePhase(engine.NewScriptedController()) // Draw

	if hand := g.Zone(engine.Hand, a).Cards(); len(hand) != 0 {
		t.Errorf("hand %v, want empty -- turn 1 draw should have been skipped", hand)
	}
}

// The skip is specific to turn 1 of a two-player game -- neither condition
// alone is enough.
func TestDrawSkipDoesNotApplyOutsideItsCase(t *testing.T) {
	t.Parallel()

	t.Run("turn 2, two players", func(t *testing.T) {
		t.Parallel()
		g := newGame(t, "a", "b")
		a, b := g.Players()[0], g.Players()[1]
		g.Player(a).Life, g.Player(b).Life = 20, 20
		g.NewCard(nil, a, engine.Library)
		g.SetTurnState(2, a, engine.Upkeep)

		g.AdvancePhase(engine.NewScriptedController())

		if hand := g.Zone(engine.Hand, a).Cards(); len(hand) != 1 {
			t.Errorf("hand %v, want one card -- turn 2 draws normally", hand)
		}
	})

	t.Run("turn 1, three players", func(t *testing.T) {
		t.Parallel()
		g := newGame(t, "a", "b", "c")
		a := g.Players()[0]
		for _, id := range g.Players() {
			g.Player(id).Life = 20
		}
		g.NewCard(nil, a, engine.Library)
		g.StartTurn(a, engine.NewScriptedController())

		g.AdvancePhase(engine.NewScriptedController()) // Upkeep
		g.AdvancePhase(engine.NewScriptedController()) // Draw

		if hand := g.Zone(engine.Hand, a).Cards(); len(hand) != 1 {
			t.Errorf("hand %v, want one card -- the skip is two-player only", hand)
		}
	})
}

// An empty library records the attempt (CR 704.5b) rather than silently
// doing nothing, and the state-based-action check that follows every phase
// entry loses the game for it.
func TestDrawFromEmptyLibraryLosesTheGame(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	a, b := g.Players()[0], g.Players()[1]
	g.Player(a).Life, g.Player(b).Life = 20, 20
	g.SetTurnState(2, a, engine.Upkeep)

	g.AdvancePhase(engine.NewScriptedController()) // -> Draw, library is empty

	if !g.Player(a).Lost {
		t.Error("player who drew from an empty library did not lose")
	}
	if !g.Over() {
		t.Error("game did not end")
	}
}

// SetTurnState is a raw jump for fixture loading: no untap, no draw.
func TestSetTurnStateRunsNoSideEffects(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	a := g.Players()[0]
	g.NewCard(nil, a, engine.Library)
	id := g.NewCard(nil, a, engine.Battlefield)
	g.Card(id).Tapped = true

	g.SetTurnState(5, a, engine.Draw)

	if g.Turn() != 5 || g.ActivePlayer() != a || g.ActivePhase() != engine.Draw {
		t.Fatalf("turn state %d/%v/%v, want 5/%v/Draw", g.Turn(), g.ActivePlayer(), g.ActivePhase(), a)
	}
	if !g.Card(id).Tapped {
		t.Error("SetTurnState untapped a permanent")
	}
	if hand := g.Zone(engine.Hand, a).Cards(); len(hand) != 0 {
		t.Error("SetTurnState drew a card")
	}
}

// Every phase entry checks state-based actions, not just Untap and Draw --
// a player already at zero life loses on the very next AdvancePhase call,
// whatever step it lands on.
func TestAdvancePhaseChecksStateBasedActionsEveryStep(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	a, b := g.Players()[0], g.Players()[1]
	g.Player(a).Life, g.Player(b).Life = 20, 20
	g.SetTurnState(1, a, engine.Main1)

	g.Player(a).Life = 0
	g.AdvancePhase(engine.NewScriptedController()) // -> CombatBegin; no combat logic runs, but the SBA check does

	if !g.Player(a).Lost {
		t.Error("player at 0 life did not lose on the next phase entry")
	}
}

// CR 514.2: cleanup clears marked damage on every permanent in the game,
// not just the active player's -- unlike untapStep, this is not scoped to
// whoever's turn it is.
func TestCleanupClearsDamageForEveryPlayer(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	a, b := g.Players()[0], g.Players()[1]
	g.Player(a).Life, g.Player(b).Life = 20, 20
	mine := g.NewCard(nil, a, engine.Battlefield)
	theirs := g.NewCard(nil, b, engine.Battlefield)
	g.Card(mine).Damage.Mark(3, false)
	g.Card(theirs).Damage.Mark(5, true)

	g.SetTurnState(1, a, engine.EndOfTurn)
	g.AdvancePhase(engine.NewScriptedController()) // -> Cleanup

	if d := g.Card(mine).Damage; d.Marked != 0 {
		t.Errorf("active player's permanent has %d damage marked after cleanup, want 0", d.Marked)
	}
	if d := g.Card(theirs).Damage; d.Marked != 0 || d.Deathtouch {
		t.Errorf("opponent's permanent damage = %+v after cleanup, want cleared", d)
	}
}

func TestCloneCopiesTurnState(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	a := g.Players()[0]
	g.StartTurn(a, engine.NewScriptedController())
	g.SetTurnState(7, a, engine.Main2)

	c := g.Clone()
	if c.Turn() != 7 || c.ActivePlayer() != a || c.ActivePhase() != engine.Main2 {
		t.Errorf("clone turn state %d/%v/%v, want 7/%v/Main2", c.Turn(), c.ActivePlayer(), c.ActivePhase(), a)
	}
}
