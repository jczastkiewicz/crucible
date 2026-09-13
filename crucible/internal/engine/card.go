// Cards. Identity and location here; the mutable detail is split out by
// concern (ADR-0009).

package engine

import (
	"github.com/jczastkiewicz/crucible/internal/carddb/compile"
	"github.com/jczastkiewicz/crucible/internal/cardtype"
	"github.com/jczastkiewicz/crucible/pkg/collect"
)

// Card is one card in one game.
//
// It holds no back-reference: no *Game, no *Player. Its controller is a
// PlayerID and its attachments are CardIDs, so a game copies with a slice copy
// and nothing has to be remapped (ADR-0009). Every operation that needs the
// rest of the game takes *Game as a parameter.
type Card struct {
	// ID is this card's own handle, so a Card passed by value still knows what
	// it is.
	ID CardID
	// Def is the compiled script, shared and immutable across every game in
	// the process (ADR-0007). Never nil for a real card.
	Def *compile.Card
	// Owner never changes. Controller does, and the two differ whenever
	// something has taken control of the card.
	Owner      PlayerID
	Controller PlayerID
	// Zone is where the card is. The zone's own list is the ordering
	// authority; this is the reverse index, kept in step by the move
	// operations.
	Zone      ZoneType
	ZoneOwner PlayerID
	// Timestamp orders continuous effects and is what breaks ties between
	// otherwise simultaneous events. Assigned from the game's counter on every
	// zone change, never reused.
	Timestamp uint64

	// The mutable detail, split by concern rather than flattened onto Card:
	// 8,105 lines of Java's Card has to land somewhere, and these are the
	// parts with their own invariants (ADR-0009).
	Counters Counters
	Damage   Damage
	Memory   Memory

	// Tapped and SummonSick are the two pieces of battlefield state every
	// permanent carries that are not "how much of something" -- everything
	// else that shape (Renowned, Monstrous, PhasedOut, and the rest of
	// GameState's per-card annotation grammar) waits on the mechanic that
	// reads it, which is card-type-specific and not built yet
	// (porting/port-log/game-state-fixture.md).
	Tapped     bool
	SummonSick bool

	// attachedTo is the card this one is attached to, and attachments is the
	// reverse. Both are unexported because they are two representations of one
	// fact and only Game.Attach and Game.Unattach may write either.
	attachedTo  CardID
	attachments *collect.OrderedSet[CardID]
}

// Type is the card's printed type line: its own primary face's, since
// CardState -- which face is current for a transformed, flipped or melded
// card -- is not modeled yet (game-state.md's "Not ported yet"), so every
// card reports the one it entered the game with. A synthetic card built
// with a nil Def (most engine tests) reports the zero Line, which matches
// nothing -- consistent with "no Def" already meaning "no name" elsewhere.
func (c *Card) Type() cardtype.Line {
	if c.Def == nil {
		return cardtype.Line{}
	}
	return c.Def.Faces[0].Type
}

// AttachedTo is what this card is attached to, and whether it is attached at
// all. Auras, Equipment and Fortifications all use it.
func (c *Card) AttachedTo() (CardID, bool) {
	if c.attachedTo == NoCard {
		return NoCard, false
	}
	return c.attachedTo, true
}

// Attachments returns what is attached to this card, in the order it was
// attached. Order decides which Aura's continuous effect applies first when
// two share a timestamp.
func (c *Card) Attachments() []CardID {
	if c.attachments == nil {
		return nil
	}
	return c.attachments.All()
}
