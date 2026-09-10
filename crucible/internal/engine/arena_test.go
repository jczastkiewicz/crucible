package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
	"github.com/jczastkiewicz/crucible/pkg/javarand"
)

func newGame(t *testing.T, players ...string) *engine.Game {
	t.Helper()
	return engine.NewGame(nil, javarand.New(1), players)
}

// The zero value of a handle field has to mean "none". Slot 0 of each arena is
// reserved for exactly that: without it, a struct that forgets to initialise a
// CardID silently points at the first card created, which is a bug that looks
// like a rules question.
func TestZeroHandleIsAbsent(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	if g.NumCards() != 0 {
		t.Errorf("new game has %d cards, want 0", g.NumCards())
	}
	if got := engine.NoCard; got != 0 {
		t.Errorf("NoCard = %d, want 0", got)
	}
	for _, id := range g.Players() {
		if id == engine.NoPlayer {
			t.Error("a real player got the NoPlayer handle")
		}
	}
}

// Handles are never reused, because last-known-information snapshots and
// remembered lists hold them after the card is gone. Reuse would alias them to
// whatever took the slot.
func TestHandlesAreNeverReused(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a")
	p := g.Players()[0]

	seen := map[engine.CardID]bool{}
	for i := 0; i < 50; i++ {
		id := g.NewCard(nil, p, engine.Library)
		if seen[id] {
			t.Fatalf("handle %d handed out twice", id)
		}
		seen[id] = true
		// Leaving the game must not free the slot.
		g.Move(id, engine.Exile, p)
	}
	if g.NumCards() != 50 {
		t.Errorf("arena holds %d handles, want 50", g.NumCards())
	}
}

// A card is in exactly one zone, and the zone's list and the card's reverse
// index have to agree. They are two representations of one fact, and a move
// that updates only one is how a card ends up in two zones at once.
func TestMoveKeepsZoneAndCardInStep(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p := g.Players()[0]
	id := g.NewCard(nil, p, engine.Library)

	if !g.Zone(engine.Library, p).Contains(id) {
		t.Fatal("new card is not in the zone it was created in")
	}
	g.Move(id, engine.Battlefield, p)

	if g.Zone(engine.Library, p).Contains(id) {
		t.Error("card is still in the zone it left")
	}
	if !g.Zone(engine.Battlefield, p).Contains(id) {
		t.Error("card is not in the zone it moved to")
	}
	if c := g.Card(id); c.Zone != engine.Battlefield || c.ZoneOwner != p {
		t.Errorf("card records zone %v/%d, want Battlefield/%d", c.Zone, c.ZoneOwner, p)
	}
}

// Every zone change takes a new timestamp. Continuous effects order by it, so
// a card that leaves and returns must not keep the ordering it had.
func TestEveryMoveStampsTheCard(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a")
	p := g.Players()[0]
	id := g.NewCard(nil, p, engine.Library)

	first := g.Card(id).Timestamp
	g.Move(id, engine.Battlefield, p)
	second := g.Card(id).Timestamp
	g.Move(id, engine.Graveyard, p)
	third := g.Card(id).Timestamp

	if first >= second || second >= third {
		t.Errorf("timestamps %d, %d, %d are not increasing", first, second, third)
	}
}

// Zone order is load-bearing: a library is a stack and a graveyard's order
// decides several counts, so insertion order is the contract.
func TestZoneKeepsInsertionOrder(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a")
	p := g.Players()[0]
	var want []engine.CardID
	for i := 0; i < 5; i++ {
		want = append(want, g.NewCard(nil, p, engine.Library))
	}
	got := g.Zone(engine.Library, p).Cards()
	if len(got) != len(want) {
		t.Fatalf("zone holds %d cards, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("position %d is card %d, want %d", i, got[i], want[i])
		}
	}
}

// An EntityID is a card or a player and must never be mistaken for the other.
// The tag is a bit, so the failure mode without a test is a player id read as
// a card id -- an index into the wrong arena.
func TestEntityTagSeparatesCardsFromPlayers(t *testing.T) {
	t.Parallel()

	c := engine.CardEntity(engine.CardID(7))
	p := engine.PlayerEntity(engine.PlayerID(7))

	if c == p {
		t.Fatal("card 7 and player 7 share an entity handle")
	}
	if id, ok := c.AsCard(); !ok || id != 7 {
		t.Errorf("card entity resolved to (%d, %v), want (7, true)", id, ok)
	}
	if _, ok := c.AsPlayer(); ok {
		t.Error("a card entity resolved as a player")
	}
	if id, ok := p.AsPlayer(); !ok || id != 7 {
		t.Errorf("player entity resolved to (%d, %v), want (7, true)", id, ok)
	}
	if _, ok := p.AsCard(); ok {
		t.Error("a player entity resolved as a card")
	}
}

// Resolving a handle the arena does not have is an engine invariant breach,
// not something a card script can cause, so it panics rather than returning an
// error (GO-7).
func TestBadHandlePanics(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a")
	for _, tc := range []struct {
		name string
		call func()
	}{
		{"NoCard", func() { g.Card(engine.NoCard) }},
		{"out of range", func() { g.Card(engine.CardID(999)) }},
		{"NoPlayer", func() { g.Player(engine.NoPlayer) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			defer func() {
				if recover() == nil {
					t.Error("resolving a bad handle did not panic")
				}
			}()
			tc.call()
		})
	}
}

// Zone names are what card scripts write, so the lookup has to be exact: a
// zone that silently resolves to None is an ability that silently does
// nothing.
func TestZoneNameLookupIsExact(t *testing.T) {
	t.Parallel()

	if z, ok := engine.ZoneByName("Battlefield"); !ok || z != engine.Battlefield {
		t.Errorf("Battlefield resolved to (%v, %v)", z, ok)
	}
	for _, bad := range []string{"battlefield", "BATTLEFIELD", " Battlefield", "Nowhere"} {
		if _, ok := engine.ZoneByName(bad); ok {
			t.Errorf("%q resolved to a zone", bad)
		}
	}
}

// The database and the RNG are injected, never reached for. Java's StaticData,
// FModel and MyRandom are process-wide singletons; translating them would put
// package-level mutable state under a goroutine-per-game engine (GO-2,
// ADR-0006), so the game hands back exactly what it was given.
func TestGameHoldsWhatItWasGiven(t *testing.T) {
	t.Parallel()

	rng := javarand.New(42)
	g := engine.NewGame(nil, rng, []string{"a"})

	if g.DB() != nil {
		t.Error("game invented a database it was not given")
	}
	if g.Rand() != rng {
		t.Error("game is not using the random stream it was given")
	}

	// Two games from the same seed produce the same stream, which is what
	// makes a batch reproducible from its seed alone.
	a := engine.NewGame(nil, javarand.New(7), []string{"a"})
	b := engine.NewGame(nil, javarand.New(7), []string{"a"})
	if a.Rand().Int32() != b.Rand().Int32() {
		t.Error("same seed produced different streams")
	}
}

// IsCard and IsPlayer are the cheap form of the accessors and have to agree
// with them, because callers use whichever reads better and a disagreement
// would be a bug that appears only on one path.
func TestEntityPredicatesAgreeWithAccessors(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name   string
		e      engine.EntityID
		card   bool
		player bool
	}{
		{"card", engine.CardEntity(3), true, false},
		{"player", engine.PlayerEntity(3), false, true},
		{"none", engine.NoEntity, false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.e.IsCard(); got != tc.card {
				t.Errorf("IsCard() = %v, want %v", got, tc.card)
			}
			if got := tc.e.IsPlayer(); got != tc.player {
				t.Errorf("IsPlayer() = %v, want %v", got, tc.player)
			}
			if _, ok := tc.e.AsCard(); ok != tc.card {
				t.Errorf("AsCard() ok = %v, want %v", ok, tc.card)
			}
			if _, ok := tc.e.AsPlayer(); ok != tc.player {
				t.Errorf("AsPlayer() ok = %v, want %v", ok, tc.player)
			}
		})
	}
}

// A zone's name round-trips, because scripts write these strings and the
// engine reports them back in events and fixtures.
func TestZoneNamesRoundTrip(t *testing.T) {
	t.Parallel()

	for _, name := range []string{"Hand", "Library", "Battlefield", "Graveyard", "Exile", "Stack", "Command", "None"} {
		z, ok := engine.ZoneByName(name)
		if !ok {
			t.Fatalf("%q is not a zone", name)
		}
		if got := z.String(); got != name {
			t.Errorf("%q round-tripped to %q", name, got)
		}
	}
	// A zone value outside the enum names itself None rather than panicking:
	// this is on the reporting path, and a report that crashes is worse than
	// one that says None.
	if got := engine.ZoneType(200).String(); got != "None" {
		t.Errorf("out-of-range zone stringified to %q, want None", got)
	}
}

// A zone is created on first use, so asking for one that no player has touched
// yet returns an empty zone rather than nil.
func TestUntouchedZoneIsEmptyNotNil(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a")
	z := g.Zone(engine.Junkyard, g.Players()[0])
	if z == nil {
		t.Fatal("zone was nil")
	}
	if z.Len() != 0 {
		t.Errorf("fresh zone holds %d cards, want 0", z.Len())
	}

	id := g.NewCard(nil, g.Players()[0], engine.Junkyard)
	if z.Len() != 1 || !z.Contains(id) {
		t.Error("the zone handed out earlier did not see the card added to it")
	}
}

// Player handles are bounds-checked on both sides, like card handles.
func TestOutOfRangePlayerPanics(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a")
	defer func() {
		if recover() == nil {
			t.Error("resolving an out-of-range player did not panic")
		}
	}()
	g.Player(engine.PlayerID(99))
}
