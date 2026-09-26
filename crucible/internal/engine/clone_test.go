package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
	"github.com/jczastkiewicz/crucible/pkg/javarand"
)

// A clone shares nothing writable with its original. Every one of these
// failures would show up as the AI's lookahead mutating the real game, which
// is the worst kind of bug to find later: the game state is wrong and nothing
// in the rules explains it.
func TestCloneSharesNothingWritable(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p := g.Players()[0]
	card := g.NewCard(nil, p, engine.Battlefield)
	aura := g.NewCard(nil, p, engine.Battlefield)

	g.Card(card).Counters.Add(engine.P1P1, 2)
	g.Card(card).Memory.Remember(engine.CardEntity(aura))
	g.Card(card).Damage.Mark(3, true)
	g.Attach(aura, card)

	c := g.Clone()

	// Zones
	c.NewCard(nil, p, engine.Battlefield)
	if g.Zone(engine.Battlefield, p).Len() == c.Zone(engine.Battlefield, p).Len() {
		t.Error("adding a card to the clone changed the original's zone")
	}

	// Counters
	c.Card(card).Counters.Add(engine.P1P1, 5)
	if got := g.Card(card).Counters.Count(engine.P1P1); got != 2 {
		t.Errorf("original's counters became %d after the clone's changed", got)
	}

	// Memory
	c.Card(card).Memory.Remember(engine.PlayerEntity(p))
	if got := len(g.Card(card).Memory.Remembered()); got != 1 {
		t.Errorf("original remembers %d things after the clone remembered one", got)
	}

	// Attachments
	c.Unattach(aura)
	if _, ok := g.Card(aura).AttachedTo(); !ok {
		t.Error("detaching in the clone detached in the original")
	}
	if len(g.Card(card).Attachments()) != 1 {
		t.Error("the original's attachment list changed with the clone's")
	}

	// Players and per-card scalars
	c.Player(p).Life = 1
	c.Card(card).Damage.Mark(4, false)
	if g.Player(p).Life == 1 {
		t.Error("the clone's life total is the original's")
	}
	if got := g.Card(card).Damage.Marked; got != 3 {
		t.Errorf("original has %d damage after the clone was damaged", got)
	}
}

// A clone continues the random stream rather than replaying it, and the two
// streams then diverge. Sharing the stream would make the AI's exploration
// change what the real game rolls.
func TestCloneTakesTheStreamWithIt(t *testing.T) {
	t.Parallel()

	g := engine.NewGame(nil, javarand.New(99), []string{"a"})
	g.Rand().Int32() // advance, so the clone is not copying a fresh stream

	c := g.Clone()
	if got, want := c.Rand().Int32(), func() int32 {
		ref := engine.NewGame(nil, javarand.New(99), []string{"a"})
		ref.Rand().Int32()
		return ref.Rand().Int32()
	}(); got != want {
		t.Errorf("clone drew %d, want %d -- it did not continue the stream", got, want)
	}

	// And drawing from the clone must not move the original.
	before := g.Rand().Int32()
	c.Rand().Int32()
	c.Rand().Int32()
	after := g.Rand().Int32()
	if before == after {
		t.Error("the two streams look identical; they may be the same object")
	}
}

// The database is shared on purpose: it is immutable and one per process, so
// copying it per clone would be the most expensive thing the AI does.
func TestCloneSharesTheDatabase(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a")
	if c := g.Clone(); c.DB() != g.DB() {
		t.Error("the clone copied the card database")
	}
}

// A clone always gets a DiscardSink, whatever the original's sink is -- the
// AI's lookahead explores lines that never happened, and a clone holding the
// real sink would record imagined casts as real.
func TestCloneAlwaysDiscardsEvents(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a")
	p := g.Players()[0]
	var sink recordingSink
	g.SetSink(&sink)

	c := g.Clone()
	c.NewCard(nil, p, engine.Hand)
	c.Move(c.Zone(engine.Hand, p).Cards()[0], engine.Graveyard, p)

	if len(sink.events) != 0 {
		t.Errorf("the original's sink saw %d events from the clone, want 0", len(sink.events))
	}
}

// Handles stay valid in the clone without being remapped, which is the whole
// reason entities are addressed by index.
func TestCloneKeepsHandlesValid(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a")
	p := g.Players()[0]
	var ids []engine.CardID
	for i := 0; i < 10; i++ {
		ids = append(ids, g.NewCard(nil, p, engine.Library))
	}
	g.Move(ids[3], engine.Graveyard, p)

	c := g.Clone()
	for _, id := range ids {
		if c.Card(id).ID != id {
			t.Fatalf("handle %d resolves to card %d in the clone", id, c.Card(id).ID)
		}
	}
	if c.Card(ids[3]).Zone != engine.Graveyard {
		t.Error("the clone lost a card's zone")
	}
	if c.NumCards() != g.NumCards() {
		t.Errorf("clone has %d handles, original %d", c.NumCards(), g.NumCards())
	}
}

// Copy cost decides how deep the AI can search, so it is measured rather than
// assumed (GO-16). The board is sized like a real mid-game one.
func BenchmarkGameClone(b *testing.B) {
	g := engine.NewGame(nil, javarand.New(1), []string{"a", "b"})
	for _, p := range g.Players() {
		for i := 0; i < 40; i++ {
			g.NewCard(nil, p, engine.Library)
		}
		for i := 0; i < 12; i++ {
			id := g.NewCard(nil, p, engine.Battlefield)
			g.Card(id).Counters.Add(engine.P1P1, 1)
		}
		for i := 0; i < 7; i++ {
			g.NewCard(nil, p, engine.Hand)
		}
	}

	b.ReportAllocs()
	for b.Loop() {
		_ = g.Clone()
	}
}

// The clone's allocation count is a gate, not a note. ADR-0009 makes copy cost
// a benchmark with a regression threshold because it decides search depth, and
// a benchmark nobody fails is decoration.
//
// Allocations rather than nanoseconds: allocation counts are deterministic
// across machines and CI runners are not, so a time bound would be either
// flaky or so loose it catches nothing. The bound has headroom over the
// measured 193 for a mid-game board -- it is here to catch a change in shape,
// like cloning something per card that used to be shared, not to police single
// allocations.
func TestCloneAllocationsStayBounded(t *testing.T) {
	// Not t.Parallel(), unlike the rest of the package (TEST-7): the
	// allocation count comes from process-wide runtime.MemStats, so any
	// parallel test allocating at the same time is counted as Clone's. A
	// sequential test runs before the parallel ones are released, which
	// isolates the measurement; under `go test -race ./...` load the parallel
	// version read 271 against this budget and 194 alone.

	g := engine.NewGame(nil, javarand.New(1), []string{"a", "b"})
	for _, p := range g.Players() {
		for i := 0; i < 40; i++ {
			g.NewCard(nil, p, engine.Library)
		}
		for i := 0; i < 12; i++ {
			id := g.NewCard(nil, p, engine.Battlefield)
			g.Card(id).Counters.Add(engine.P1P1, 1)
		}
		for i := 0; i < 7; i++ {
			g.NewCard(nil, p, engine.Hand)
		}
	}

	const budget = 260
	res := testing.Benchmark(func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			_ = g.Clone()
		}
	})
	t.Logf("Game.Clone: %d allocs per copy", res.AllocsPerOp())
	if got := res.AllocsPerOp(); got > budget {
		t.Errorf("Game.Clone allocates %d times per copy, budget %d -- "+
			"something that was shared is now copied per card", got, budget)
	}
}

// A clone keeps allocating stack item IDs where the original left off, so
// its next push never reuses an ID already on its stack (ADR-0018: a
// StackItemID names one stack object for the whole game, and a copy of a
// spell is told from its original by it). Regression: Clone did not carry
// the counter, so a clone's first push reused ID 1.
func TestCloneContinuesStackItemIDs(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p := g.Players()[0]
	g.PushAbility(engine.Ability{Source: 1, Controller: p})
	g.PushAbility(engine.Ability{Source: 2, Controller: p})
	second, _ := g.StackTop()

	clone := g.Clone()
	clone.PushAbility(engine.Ability{Source: 3, Controller: p})
	third, _ := clone.StackTop()
	if third.ID <= second.ID {
		t.Errorf("clone's next StackItemID = %d, want past the original's %d", third.ID, second.ID)
	}
}
