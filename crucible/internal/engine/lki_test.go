package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
)

// TestLKIAbsentBeforeLeavingBattlefield proves Game.LKI (game.go) reports
// nothing for a card that has never left the battlefield -- Move's own
// battlefield-leaving branch is the only writer, so a card still on the
// battlefield (or one that never entered one at all) has no snapshot yet.
func TestLKIAbsentBeforeLeavingBattlefield(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a")
	p := g.Players()[0]
	id := g.NewCard(creatureDefPT(t, "2", "2"), p, engine.Battlefield)

	if snap := g.LKI(id); snap != nil {
		t.Fatalf("LKI(%v) = %+v, want nil before the card ever leaves the battlefield", id, snap)
	}
}

// TestLKIFreezesStateAtTheMomentOfLeaving proves Move (game.go) captures the
// snapshot before, not after, clearing Counters/PT/TypeMod/ColorMod/
// KeywordMod -- Game.LKI's own doc comment names this the whole point: a
// caller consulting it after the move still sees the card as it stood on the
// battlefield an instant earlier, not the printed-only state the live card
// has already been reset to.
func TestLKIFreezesStateAtTheMomentOfLeaving(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a")
	p := g.Players()[0]
	id := g.NewCard(creatureDefPT(t, "2", "2"), p, engine.Battlefield)
	g.Card(id).Counters.Add(engine.P1P1, 3)

	g.Move(id, engine.Graveyard, p)

	if got := g.Card(id).Counters.Count(engine.P1P1); got != 0 {
		t.Fatalf("live card has %d P1P1 counters after leaving, want 0 (Move's own clear)", got)
	}
	snap := g.LKI(id)
	if snap == nil {
		t.Fatal("LKI() = nil after leaving the battlefield, want a snapshot")
	}
	if got := snap.Counters.Count(engine.P1P1); got != 3 {
		t.Errorf("LKI() P1P1 counters = %d, want 3 (the count at the moment of leaving)", got)
	}
	if pw, ok := snap.Power(); !ok || pw != 5 {
		t.Errorf("LKI() Power() = (%d, %v), want (5, true) -- 2 base + 3 P1P1", pw, ok)
	}
}

// TestLKIOverwrittenOnEachSubsequentLeave proves the snapshot is replaced
// whole, never merged, matching CardCopyService.getLKICopy()'s own "one
// frozen copy" contract (Game.lki's own doc comment, game.go): a card
// returned to the battlefield and then leaving again gets a fresh snapshot
// reflecting its state at the SECOND departure, not a stale mix of both.
func TestLKIOverwrittenOnEachSubsequentLeave(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a")
	p := g.Players()[0]
	id := g.NewCard(creatureDefPT(t, "2", "2"), p, engine.Battlefield)
	g.Card(id).Counters.Add(engine.P1P1, 1)
	g.Move(id, engine.Graveyard, p)

	g.Move(id, engine.Battlefield, p)
	g.Card(id).Counters.Add(engine.P1P1, 4)
	g.Move(id, engine.Graveyard, p)

	if got := g.LKI(id).Counters.Count(engine.P1P1); got != 4 {
		t.Errorf("LKI() P1P1 counters = %d after a second departure, want 4, not a mix with the first", got)
	}
}

// TestCloneCopiesLKI proves Game.Clone (game.go) gives the clone its own
// independent LKI snapshot, the same "shares nothing writable" contract
// TestCloneCopiesPT/TestCloneCopiesCombat/... already hold for every other
// per-card ledger.
func TestCloneCopiesLKI(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a")
	p := g.Players()[0]
	id := g.NewCard(creatureDefPT(t, "2", "2"), p, engine.Battlefield)
	g.Card(id).Counters.Add(engine.P1P1, 1)
	g.Move(id, engine.Graveyard, p)

	c := g.Clone()
	c.LKI(id).Counters.Add(engine.P1P1, 5)

	if got := g.LKI(id).Counters.Count(engine.P1P1); got != 1 {
		t.Errorf("original LKI() counters = %d after the clone's changed, want 1", got)
	}
	if got := c.LKI(id).Counters.Count(engine.P1P1); got != 6 {
		t.Errorf("clone LKI() counters = %d, want 6", got)
	}
}
