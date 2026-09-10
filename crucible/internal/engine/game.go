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
}

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
func (g *Game) Move(id CardID, kind ZoneType, owner PlayerID) {
	c := g.Card(id)
	g.Zone(c.Zone, c.ZoneOwner).cards.Remove(id)
	g.put(id, kind, owner)
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
