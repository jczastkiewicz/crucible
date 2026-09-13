package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
)

// Leaving the battlefield clears everything Java gets for free by building a
// new Card object for the destination zone: counters, damage, tapped, and
// what this card itself was attached to.
func TestMoveClearsBattlefieldStateOnLeaving(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a")
	p := g.Players()[0]
	host := g.NewCard(nil, p, engine.Battlefield)
	aura := g.NewCard(nil, p, engine.Battlefield)

	c := g.Card(aura)
	c.Counters.Add(engine.P1P1, 3)
	c.Damage.Mark(2, false)
	c.Tapped = true
	g.Attach(aura, host)

	g.Move(aura, engine.Graveyard, p)

	if got := c.Counters.Count(engine.P1P1); got != 0 {
		t.Errorf("counters %d after leaving the battlefield, want 0", got)
	}
	if c.Damage.Marked != 0 {
		t.Errorf("damage %d after leaving the battlefield, want 0", c.Damage.Marked)
	}
	if c.Tapped {
		t.Error("still tapped after leaving the battlefield")
	}
	if _, ok := c.AttachedTo(); ok {
		t.Error("the aura is still attached to its host after leaving the battlefield")
	}
}

// A card attached to the leaving card is untouched -- only the leaving
// card's own attachment breaks. Cleaning up a dangling AttachedTo on the
// other side is a state-based action this port has not built yet.
func TestMoveLeavesWhatWasAttachedToItAlone(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a")
	p := g.Players()[0]
	host := g.NewCard(nil, p, engine.Battlefield)
	aura := g.NewCard(nil, p, engine.Battlefield)
	g.Attach(aura, host)

	g.Move(host, engine.Graveyard, p)

	target, ok := g.Card(aura).AttachedTo()
	if !ok || target != host {
		t.Errorf("aura attachment changed to (%v, %v) after its host left, want (%v, true) -- unchanged", target, ok, host)
	}
}

// Entering the battlefield sets SummonSick, the same as a freshly-built Java
// Card object starts sick unless something clears it.
func TestMoveSetsSummonSickOnEnteringBattlefield(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a")
	p := g.Players()[0]
	id := g.NewCard(nil, p, engine.Library)

	g.Move(id, engine.Battlefield, p)

	if !g.Card(id).SummonSick {
		t.Error("card entering the battlefield is not summoning sick")
	}
}

// A card that dies and returns is freshly sick and clean, not a survivor of
// its last trip: this is the round trip the two halves of Move exist for.
func TestMoveRoundTripLeavesNoResidue(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a")
	p := g.Players()[0]
	id := g.NewCard(nil, p, engine.Battlefield)
	c := g.Card(id)
	c.Counters.Add(engine.P1P1, 1)
	c.SummonSick = false // this permanent has been out since last turn

	g.Move(id, engine.Graveyard, p)
	g.Move(id, engine.Battlefield, p)

	if got := c.Counters.Count(engine.P1P1); got != 0 {
		t.Errorf("counters %d on return, want 0", got)
	}
	if !c.SummonSick {
		t.Error("card returning to the battlefield is not summoning sick")
	}
}

// Neither zone in the move is the battlefield, so nothing here applies --
// the switch's two cases are mutually exclusive on purpose, not just in
// practice.
func TestMoveBetweenNonBattlefieldZonesTouchesNoBattlefieldState(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a")
	p := g.Players()[0]
	id := g.NewCard(nil, p, engine.Hand)

	g.Move(id, engine.Graveyard, p)

	c := g.Card(id)
	if c.Tapped || c.SummonSick || c.Counters.Any() || c.Damage.Marked != 0 {
		t.Error("a hand-to-graveyard move touched battlefield-only state")
	}
}
