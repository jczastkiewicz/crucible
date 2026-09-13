// The game: the arena every handle indexes into, and the zone table.

// Package engine is the rules core.
//
// It is one Go package on purpose. Java's forge.game splits across 21 packages
// with 82 two-package cycles between them, and Go forbids cycles, so the
// mirrored layout does not compile. What lives here is the set of types whose
// references are genuinely mutual -- Game, Card, Player, Zone, and the stack,
// combat and effect machinery that lands in later slices. Everything that can
// be acyclic is a separate package importing this one, never the other way
// round (ADR-0003).
//
// Inside the package, tools/enginelint enforces the file-group boundaries the
// compiler cannot see, because Go has no sub-package visibility.
package engine

import (
	"github.com/jczastkiewicz/crucible/internal/carddb/compile"
	"github.com/jczastkiewicz/crucible/pkg/collect"
	"github.com/jczastkiewicz/crucible/pkg/javarand"
)

// Game is one game in progress. It owns every entity in it.
//
// The engine runs a goroutine per game and games share only immutable data, so
// nothing here is guarded and nothing here should be: a mutex in this package
// is a design bug (GO-3, ADR-0005).
type Game struct {
	// cards is the arena. A CardID is an index into it, slot 0 is NoCard, and
	// the slice only ever grows -- handles are never reused (ADR-0009).
	cards []Card
	// players is the arena for players, on the same terms.
	players []Player
	// zones is keyed by type and owner. Ownerless zones -- the stack, and the
	// shared exile Java models per-player -- use NoPlayer.
	zones map[zoneKey]*Zone

	// db is the compiled card database, shared read-only by every game in the
	// process (ADR-0005, ADR-0007).
	db *compile.DB
	// rand is this game's own stream, seeded per game so a run is
	// reproducible from its seed alone. There is no package-level RNG
	// (GO-2, ADR-0006).
	rand *javarand.Rand

	// timestamp is the monotonic counter behind Card.Timestamp. It only ever
	// increases, so an ordering never repeats within a game.
	timestamp uint64

	// over is set once CheckStateBasedActions decides the game has ended --
	// a win, a loss, or a draw. Nothing unsets it: a game that has ended
	// stays ended (action.go).
	over bool

	// turn, activePlayer and activePhase are the turn structure: whose turn
	// it is, what step or phase it is in, and how many turns have passed.
	// Zero-valued (0, NoPlayer, Untap) until StartTurn, which is also a
	// coherent "the game has not started its turn structure yet" reading --
	// nothing downstream treats an unstarted game's phase as meaningful
	// without also checking activePlayer (turn.go).
	turn         int
	activePlayer PlayerID
	activePhase  PhaseType

	// sink is where this game's events go. DiscardSink by default: most
	// callers -- every test, fixture loading -- have nothing listening and
	// should not have to construct a sink just to build a game.
	sink Sink
}

// SetSink replaces the game's event sink. The zero Game has a DiscardSink,
// so this is opt-in for whatever eventually reads the stream (a recorder,
// M8) rather than a constructor parameter every existing caller would have
// had to grow one to keep compiling.
func (g *Game) SetSink(s Sink) { g.sink = s }

// Over reports whether the game has ended, per the last call to
// CheckStateBasedActions.
func (g *Game) Over() bool { return g.over }

// Turn is the current turn number, per the last player whose turn ended.
func (g *Game) Turn() int { return g.turn }

// ActivePlayer is whose turn it is. NoPlayer before StartTurn.
func (g *Game) ActivePlayer() PlayerID { return g.activePlayer }

// ActivePhase is the step or phase in progress.
func (g *Game) ActivePhase() PhaseType { return g.activePhase }

// zoneKey identifies a zone. Ownerless zones carry NoPlayer.
type zoneKey struct {
	kind  ZoneType
	owner PlayerID
}

// NewGame builds an empty game with the given players.
//
// The database and the RNG are injected rather than reached for, which is what
// keeps this package free of the singletons Java uses -- StaticData, FModel
// and MyRandom all become parameters (GO-2).
func NewGame(db *compile.DB, rng *javarand.Rand, names []string) *Game {
	g := &Game{
		// Slot 0 is NoCard and NoPlayer, so a zero-valued handle field means
		// "none" instead of aliasing the first entity created.
		cards:   make([]Card, 1, 128),
		players: make([]Player, 1, len(names)+1),
		zones:   make(map[zoneKey]*Zone, len(names)*8),
		db:      db,
		rand:    rng,
		sink:    DiscardSink{},
	}
	for _, name := range names {
		id := PlayerID(len(g.players))
		g.players = append(g.players, Player{ID: id, Name: name})
		for _, z := range []ZoneType{Hand, Library, Graveyard, Battlefield, Exile, Command, Sideboard} {
			g.zones[zoneKey{z, id}] = &Zone{Type: z, Owner: id, cards: collect.NewOrderedSet[CardID](0)}
		}
	}
	g.zones[zoneKey{Stack, NoPlayer}] = &Zone{Type: Stack, cards: collect.NewOrderedSet[CardID](0)}
	return g
}

// DB is the compiled card database this game reads from.
func (g *Game) DB() *compile.DB { return g.db }

// Rand is this game's random stream.
func (g *Game) Rand() *javarand.Rand { return g.rand }

// Card resolves a handle. It panics on an out-of-range or absent handle,
// because that is an engine invariant breach rather than anything a card
// script can cause -- the game boundary recovers it so one bad card cannot
// take down a batch (GO-7).
func (g *Game) Card(id CardID) *Card {
	if id == NoCard || int(id) >= len(g.cards) {
		panic("engine: card handle out of range")
	}
	return &g.cards[id]
}

// Player resolves a player handle, on the same terms as [Game.Card].
func (g *Game) Player(id PlayerID) *Player {
	if id == NoPlayer || int(id) >= len(g.players) {
		panic("engine: player handle out of range")
	}
	return &g.players[id]
}

// Players returns every player, in seating order.
func (g *Game) Players() []PlayerID {
	out := make([]PlayerID, 0, len(g.players)-1)
	for i := 1; i < len(g.players); i++ {
		out = append(out, PlayerID(i))
	}
	return out
}

// NumCards is how many handles have been allocated, NoCard excluded. It is the
// arena's high-water mark, not a count of cards in play.
func (g *Game) NumCards() int { return len(g.cards) - 1 }

// NewCard allocates a card and puts it in a zone.
//
// The handle it returns is stable for the life of the game even after the card
// changes zone, is destroyed, or leaves the game entirely.
func (g *Game) NewCard(def *compile.Card, owner PlayerID, zone ZoneType) CardID {
	id := CardID(len(g.cards))
	g.cards = append(g.cards, Card{
		ID:         id,
		Def:        def,
		Owner:      owner,
		Controller: owner,
	})
	g.put(id, zone, owner)
	return id
}

// Zone returns a zone by type and owner, creating it on first use. Ownerless
// zones are addressed with NoPlayer.
func (g *Game) Zone(kind ZoneType, owner PlayerID) *Zone {
	key := zoneKey{kind, owner}
	z, ok := g.zones[key]
	if !ok {
		z = &Zone{Type: kind, Owner: owner, cards: collect.NewOrderedSet[CardID](0)}
		g.zones[key] = z
	}
	return z
}

// Move takes a card out of the zone it is in and appends it to another,
// stamping it on the way.
//
// Every zone change gets a new timestamp, which is what continuous effects
// order by and what makes a card that left and came back a different object to
// the layer system.
//
// Leaving the battlefield clears Counters, Damage, Tapped and any
// attachment; entering it sets SummonSick. Java gets both for free:
// GameAction.changeZone builds a new Card object for the destination zone
// (CardCopyService.copyCard), so a field simply is not copied onto it, and a
// freshly-built permanent starts sick unless something says otherwise. A
// CardID is stable across zone changes here instead (ADR-0009) — the same
// struct persists, so a creature that dies with three +1/+1 counters would
// return from the graveyard still carrying them unless this clears them, and
// a Raise Dead'd creature would enter without summoning sickness unless this
// sets it.
//
// [Game.NewCard] does neither: it is the arena-allocation primitive fixture
// loading uses to seat a board mid-game, where a battlefield permanent's
// starting Tapped/SummonSick is exactly what the fixture says, not a rule
// this port applies. Move is real play transitioning a card between zones;
// NewCard is "this card already exists here."
//
// Every call emits a ZoneChanged event, for the same reason NewCard does
// not: this is real play, and NewCard is setup nothing downstream should
// see as something happening.
func (g *Game) Move(id CardID, kind ZoneType, owner PlayerID) {
	c := g.Card(id)
	from := c.Zone
	g.Zone(c.Zone, c.ZoneOwner).cards.Remove(id)
	g.put(id, kind, owner)

	switch {
	case from == Battlefield && kind != Battlefield:
		c.Counters = Counters{}
		c.Damage.Clear()
		c.Tapped = false
		c.SummonSick = false
		g.Unattach(id)
	case from != Battlefield && kind == Battlefield:
		c.SummonSick = true
	}

	g.sink.Emit(Event{
		Kind:   ZoneChanged,
		Phase:  g.activePhase,
		Active: g.activePlayer,
		Actor:  owner,
		Turn:   uint16(g.turn),
		Source: id,
		From:   from,
		To:     kind,
	})
}

// Shuffle randomises one zone's order, in place, using the game's own random
// stream. Ported from Player.shuffle (Collections.shuffle(list,
// MyRandom.getRandom())): javarand.Rand.Shuffle reproduces that algorithm
// exactly, which is what makes a shuffled library replay identically from
// the same seed (pkg/javarand's P0 gate).
//
// A shuffle stamps no timestamp and fires no zone-change: order within a zone
// is not itself a zone change, and nothing reads a card's Timestamp to learn
// where it sits in its own library.
func (g *Game) Shuffle(kind ZoneType, owner PlayerID) {
	z := g.Zone(kind, owner)
	g.rand.Shuffle(z.cards.Len(), z.cards.Swap)
}

// put appends a card to a zone and records the reverse index on the card. It
// does not remove the card from wherever it was, so only [Game.Move] and
// [Game.NewCard] may call it.
func (g *Game) put(id CardID, kind ZoneType, owner PlayerID) {
	c := &g.cards[id]
	c.Zone, c.ZoneOwner = kind, owner
	g.timestamp++
	c.Timestamp = g.timestamp
	g.Zone(kind, owner).cards.Add(id)
}

// Attach attaches one card to another, moving it off whatever it was attached
// to first.
//
// The two sides -- the attachment's own pointer and the host's list -- are one
// fact stored twice, so this and [Game.Unattach] are the only writers. A card
// cannot be attached to itself, and cannot be attached to a card that does not
// exist; both are invariant breaches rather than rules questions (GO-7).
func (g *Game) Attach(attachment, host CardID) {
	if attachment == host {
		panic("engine: card attached to itself")
	}
	a, h := g.Card(attachment), g.Card(host)
	g.Unattach(attachment)
	a.attachedTo = host
	if h.attachments == nil {
		h.attachments = collect.NewOrderedSet[CardID](2)
	}
	h.attachments.Add(attachment)
}

// Unattach detaches a card from whatever it is attached to. Detaching an
// unattached card is a no-op, because the callers that clean up after a zone
// change do not track whether there was anything to clean.
func (g *Game) Unattach(attachment CardID) {
	a := g.Card(attachment)
	if a.attachedTo == NoCard {
		return
	}
	if h := g.Card(a.attachedTo); h.attachments != nil {
		h.attachments.Remove(attachment)
	}
	a.attachedTo = NoCard
}

// Clone returns an independent copy of the game.
//
// This is what the AI's lookahead runs on, so it is on a hot path and its cost
// decides search depth (GO-16). Handles are indices, so nothing has to be
// remapped: there is no equivalent of Java's CopiedGameObjectMap, and that is
// the point of addressing entities by handle (ADR-0009).
//
// "A slice copy" is the shape but not the whole job. A Card owns collections
// behind pointers -- its counters, its three memory lists, its attachments --
// and copying the slice alone would leave the clone and the original writing
// to the same ones. Each is copied when it exists and left nil when it does
// not, which is most cards most of the time.
//
// The database is shared, because it is immutable (ADR-0005). The random
// stream is copied by value, so the clone continues from where the original
// is rather than replaying it or advancing it.
func (g *Game) Clone() *Game {
	out := &Game{
		cards:        make([]Card, len(g.cards)),
		players:      append([]Player(nil), g.players...),
		zones:        make(map[zoneKey]*Zone, len(g.zones)),
		db:           g.db,
		timestamp:    g.timestamp,
		over:         g.over,
		turn:         g.turn,
		activePlayer: g.activePlayer,
		activePhase:  g.activePhase,
		// Always DiscardSink, whatever the original's sink is: the AI's
		// lookahead explores lines that never happened, and a clone holding
		// the real sink would record imagined casts as real.
		sink: DiscardSink{},
	}
	if g.rand != nil {
		r := *g.rand
		out.rand = &r
	}

	for i := range out.players {
		out.players[i].Counters = g.players[i].Counters.clone()
	}

	copy(out.cards, g.cards)
	for i := range out.cards {
		c := &out.cards[i]
		c.Counters = g.cards[i].Counters.clone()
		c.Memory = g.cards[i].Memory.clone()
		if g.cards[i].attachments != nil {
			c.attachments = g.cards[i].attachments.Clone()
		}
	}
	for k, z := range g.zones {
		out.zones[k] = &Zone{Type: z.Type, Owner: z.Owner, cards: z.cards.Clone()}
	}
	return out
}
