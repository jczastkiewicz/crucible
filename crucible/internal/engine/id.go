// Handles. Identity in the engine is an integer, never a pointer.

package engine

// CardID addresses a card in its game's arena. It is the index, so a lookup is
// a slice index and a comparison is `==` (GO-9, ADR-0009).
//
// Handles are never reused within a game. A card that leaves the battlefield
// keeps its slot, because last-known-information snapshots, `Remembered` lists
// and delayed triggers all hold handles to objects that are gone, and reusing
// a slot would silently alias them to something else.
type CardID uint32

// NoCard is the absent card. Slot 0 of every arena is reserved for it, so the
// zero value of a CardID field means "none" rather than "the first card ever
// created" -- a distinction that is otherwise a bug per struct.
const NoCard CardID = 0

// PlayerID addresses a player. Games are two-player by default and Commander
// runs to six, so a byte is not a gamble.
type PlayerID uint8

// NoPlayer is the absent player, on the same reasoning as [NoCard].
const NoPlayer PlayerID = 0

// EntityID is a card or a player, for the places the rules treat them
// interchangeably: targeting, damage, and the `Defined$` vocabulary.
//
// The tag is the high bit rather than a struct with a kind, because an
// EntityID is stored in the thousands -- in target lists, remembered lists and
// LKI snapshots -- and a 4-byte handle that compares with `==` is the point of
// ADR-0009.
type EntityID uint32

// entityPlayer tags an EntityID as a player. Cards keep the low range, so a
// card's EntityID is numerically its CardID.
const entityPlayer EntityID = 1 << 31

// NoEntity is the absent entity.
const NoEntity EntityID = 0

// CardEntity is the entity handle for a card.
func CardEntity(id CardID) EntityID { return EntityID(id) }

// PlayerEntity is the entity handle for a player.
func PlayerEntity(id PlayerID) EntityID { return entityPlayer | EntityID(id) }

// AsCard returns the card this entity is, and whether it is one.
func (e EntityID) AsCard() (CardID, bool) {
	if e == NoEntity || e&entityPlayer != 0 {
		return NoCard, false
	}
	return CardID(e), true
}

// AsPlayer returns the player this entity is, and whether it is one.
func (e EntityID) AsPlayer() (PlayerID, bool) {
	if e&entityPlayer == 0 {
		return NoPlayer, false
	}
	return PlayerID(e &^ entityPlayer), true
}

// IsCard reports whether the entity is a card.
func (e EntityID) IsCard() bool { _, ok := e.AsCard(); return ok }

// IsPlayer reports whether the entity is a player.
func (e EntityID) IsPlayer() bool { _, ok := e.AsPlayer(); return ok }
