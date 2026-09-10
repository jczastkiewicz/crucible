package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
)

// A counter type with no counters is absent, not zero. "A permanent with a
// counter of any kind on it" has to say no for a card that gained and lost a
// counter, and a stored zero would say yes.
func TestCountersAtZeroAreAbsent(t *testing.T) {
	t.Parallel()

	var c engine.Counters
	if c.Any() || c.Total() != 0 || len(c.Kinds()) != 0 {
		t.Fatal("a fresh Counters is not empty")
	}

	c.Add(engine.P1P1, 2)
	if !c.Any() || c.Count(engine.P1P1) != 2 {
		t.Fatalf("after adding 2: any=%v count=%d", c.Any(), c.Count(engine.P1P1))
	}

	c.Add(engine.P1P1, -2)
	if c.Any() {
		t.Error("a card that lost its last counter still reports having one")
	}
	if kinds := c.Kinds(); len(kinds) != 0 {
		t.Errorf("kinds after removal: %v, want none", kinds)
	}
}

// Removing more counters than the card has removes what is there, and does not
// leave a negative count behind for the next arithmetic to read.
func TestCountersDoNotGoNegative(t *testing.T) {
	t.Parallel()

	var c engine.Counters
	c.Add(engine.Charge, 1)
	if got := c.Add(engine.Charge, -5); got != 0 {
		t.Errorf("removing 5 of 1 left %d, want 0", got)
	}
	if got := c.Count(engine.Charge); got != 0 {
		t.Errorf("count is %d after over-removal, want 0", got)
	}
}

// Counter kinds come back in the order they were first put on, and changing a
// count does not reorder them: a report and an event stream have to look the
// same on every run (GO-12).
func TestCounterOrderIsStable(t *testing.T) {
	t.Parallel()

	var c engine.Counters
	c.Add(engine.Stun, 1)
	c.Add(engine.P1P1, 1)
	c.Add(engine.Shield, 1)
	c.Add(engine.Stun, 4) // an update, not a reinsertion

	want := []engine.CounterType{engine.Stun, engine.P1P1, engine.Shield}
	got := c.Kinds()
	if len(got) != len(want) {
		t.Fatalf("kinds %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("position %d is %q, want %q", i, got[i], want[i])
		}
	}
	if c.Total() != 7 {
		t.Errorf("total is %d, want 7", c.Total())
	}
}

// A deathtouch source sets a flag that later non-deathtouch damage must not
// clear: the state-based action checks the flag, not the amount.
func TestDeathtouchFlagSticksForTheTurn(t *testing.T) {
	t.Parallel()

	var d engine.Damage
	d.Mark(1, true)
	d.Mark(3, false)

	if !d.Deathtouch {
		t.Error("a later ordinary point of damage cleared the deathtouch flag")
	}
	if d.Marked != 4 {
		t.Errorf("marked %d, want 4", d.Marked)
	}

	d.Clear()
	if d.Marked != 0 || d.Deathtouch || d.ExcessThisTurn {
		t.Errorf("cleanup left %+v", d)
	}
}

// Zero or negative damage is not damage. Several effects compute an amount
// that can come out at zero, and marking it would set the deathtouch flag for
// a source that dealt nothing.
func TestZeroDamageIsNotDealt(t *testing.T) {
	t.Parallel()

	var d engine.Damage
	d.Mark(0, true)
	d.Mark(-3, true)
	if d.Marked != 0 || d.Deathtouch {
		t.Errorf("marking nothing left %+v", d)
	}
}

// Remembered holds entities, because RememberObjects$ puts players and cards
// in one list. Order is the order they were remembered, which is what a
// RepeatEach$ over the list walks.
func TestMemoryRemembersCardsAndPlayersInOrder(t *testing.T) {
	t.Parallel()

	var m engine.Memory
	first := engine.CardEntity(4)
	second := engine.PlayerEntity(2)

	if !m.Remember(first) || !m.Remember(second) {
		t.Fatal("Remember reported a new entity as already present")
	}
	if m.Remember(first) {
		t.Error("Remember reported a duplicate as new")
	}

	got := m.Remembered()
	if len(got) != 2 || got[0] != first || got[1] != second {
		t.Fatalf("remembered %v, want [%v %v]", got, first, second)
	}
	if _, ok := got[1].AsPlayer(); !ok {
		t.Error("the player came back as something else")
	}

	m.ClearRemembered()
	if len(m.Remembered()) != 0 {
		t.Error("ClearRemembered left something behind")
	}
}

// The three lists are separate. A cleanup that clears one must not empty the
// others, which is why card scripts spell out ClearRemembered, ClearImprinted
// and ClearChosenCard independently.
func TestMemoryListsAreIndependent(t *testing.T) {
	t.Parallel()

	var m engine.Memory
	m.Remember(engine.CardEntity(1))
	m.Imprint(engine.CardID(2))
	m.Choose(engine.CardID(3))

	m.ClearRemembered()
	if len(m.Imprinted()) != 1 || len(m.Chosen()) != 1 {
		t.Error("clearing remembered emptied another list")
	}
	m.ClearImprinted()
	if len(m.Chosen()) != 1 {
		t.Error("clearing imprinted emptied the chosen list")
	}
	m.ClearChosen()
	if len(m.Chosen()) != 0 {
		t.Error("ClearChosen left something behind")
	}
}

// Attachment is one fact stored twice. Attaching has to write both sides, and
// re-attaching elsewhere has to leave the old host's list clean.
func TestAttachKeepsBothSidesInStep(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a")
	p := g.Players()[0]
	aura := g.NewCard(nil, p, engine.Battlefield)
	first := g.NewCard(nil, p, engine.Battlefield)
	second := g.NewCard(nil, p, engine.Battlefield)

	g.Attach(aura, first)
	if host, ok := g.Card(aura).AttachedTo(); !ok || host != first {
		t.Fatalf("aura attached to (%d, %v), want (%d, true)", host, ok, first)
	}
	if got := g.Card(first).Attachments(); len(got) != 1 || got[0] != aura {
		t.Fatalf("host's attachments %v, want [%d]", got, aura)
	}

	g.Attach(aura, second)
	if got := g.Card(first).Attachments(); len(got) != 0 {
		t.Errorf("the old host still lists the aura: %v", got)
	}
	if got := g.Card(second).Attachments(); len(got) != 1 || got[0] != aura {
		t.Errorf("the new host lists %v, want [%d]", got, aura)
	}

	g.Unattach(aura)
	if _, ok := g.Card(aura).AttachedTo(); ok {
		t.Error("the aura is still attached after Unattach")
	}
	if got := g.Card(second).Attachments(); len(got) != 0 {
		t.Errorf("the host still lists a detached aura: %v", got)
	}

	// Detaching something that is not attached is a no-op, because the zone
	// change cleanup does not track whether there was anything to clean.
	g.Unattach(aura)
}

// A card attached to itself would make the attachment walk cycle, so it is an
// invariant breach rather than a rules question.
func TestSelfAttachPanics(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a")
	p := g.Players()[0]
	id := g.NewCard(nil, p, engine.Battlefield)

	defer func() {
		if recover() == nil {
			t.Error("attaching a card to itself did not panic")
		}
	}()
	g.Attach(id, id)
}
