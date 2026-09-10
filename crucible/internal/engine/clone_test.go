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
	t.Parallel()

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
	if got := res.AllocsPerOp(); got > budget {
		t.Errorf("Game.Clone allocates %d times per copy, budget %d -- "+
			"something that was shared is now copied per card", got, budget)
	}
}
