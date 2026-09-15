package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
)

// Every Move emits a ZoneChanged event naming the card, both zones, and the
// game's turn/phase/active context at the time -- a recorder should not have
// to cross-reference a separate log to know when a move happened.
func TestMoveEmitsZoneChanged(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p := g.Players()[0]
	var sink recordingSink
	g.SetSink(&sink)
	g.SetTurnState(3, p, engine.Main1)
	id := g.NewCard(nil, p, engine.Hand)

	g.Move(id, engine.Graveyard, p)

	if len(sink.events) != 1 {
		t.Fatalf("sink saw %d events, want 1", len(sink.events))
	}
	e := sink.events[0]
	if e.Kind != engine.ZoneChanged {
		t.Errorf("kind %s, want ZoneChanged", e.Kind)
	}
	if e.Source != id {
		t.Errorf("source %v, want %v", e.Source, id)
	}
	if e.From != engine.Hand || e.To != engine.Graveyard {
		t.Errorf("from/to %v/%v, want Hand/Graveyard", e.From, e.To)
	}
	if e.Actor != p {
		t.Errorf("actor %v, want %v", e.Actor, p)
	}
	if e.Turn != 3 || e.Phase != engine.Main1 {
		t.Errorf("turn/phase %d/%v, want 3/Main1", e.Turn, e.Phase)
	}
}

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

// Entering the battlefield gives a planeswalker its printed starting
// loyalty as counters (CR 121.5) -- the same "Java gets this for free by
// building a new Card object" gap SummonSick's own test closes for combat.
func TestMoveGrantsAPlaneswalkerItsStartingLoyalty(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a")
	p := g.Players()[0]
	id := g.NewCard(planeswalkerDefLoyalty(t, "5"), p, engine.Hand)

	g.Move(id, engine.Battlefield, p)

	if got := g.Card(id).Counters.Count(engine.Loyalty); got != 5 {
		t.Errorf("loyalty counters after entering the battlefield = %d, want 5", got)
	}
}

// Entering the battlefield gives a Battle its printed starting defense as
// counters (CR 704.5v) -- Loyalty's own counterpart.
func TestMoveGrantsABattleItsStartingDefense(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a")
	p := g.Players()[0]
	id := g.NewCard(battleDefDefense(t, "5"), p, engine.Hand)

	g.Move(id, engine.Battlefield, p)

	if got := g.Card(id).Counters.Count(engine.Defense); got != 5 {
		t.Errorf("defense counters after entering the battlefield = %d, want 5", got)
	}
}

// An ordinary creature entering the battlefield gets neither counter kind --
// the ETB grant is specific to what a planeswalker or a Battle's loyalty/
// defense actually is, not a blanket "starting counters" rule.
func TestMoveGrantsNoStartingCountersToAnOrdinaryCreature(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a")
	p := g.Players()[0]
	id := g.NewCard(creatureDef(t), p, engine.Hand)

	g.Move(id, engine.Battlefield, p)

	c := g.Card(id)
	if got := c.Counters.Count(engine.Loyalty); got != 0 {
		t.Errorf("a creature entering the battlefield has %d loyalty counters, want 0", got)
	}
	if got := c.Counters.Count(engine.Defense); got != 0 {
		t.Errorf("a creature entering the battlefield has %d defense counters, want 0", got)
	}
}

// A planeswalker that dies and re-enters gets a fresh loyalty grant, not
// whatever count it happened to have (zero, since dying cleared it) --
// entering the battlefield is entering as a new object every time.
func TestMovePlaneswalkerReenteringGetsFreshLoyalty(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a")
	p := g.Players()[0]
	id := g.NewCard(planeswalkerDefLoyalty(t, "4"), p, engine.Battlefield)
	g.Card(id).Counters.Add(engine.Loyalty, 4)
	g.Card(id).Counters.Add(engine.Loyalty, -4) // paid down to 0, as if by an ability

	g.Move(id, engine.Graveyard, p)
	g.Move(id, engine.Battlefield, p)

	if got := g.Card(id).Counters.Count(engine.Loyalty); got != 4 {
		t.Errorf("loyalty counters after re-entering the battlefield = %d, want 4", got)
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
