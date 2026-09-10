// Zones, and the ordered card lists they hold.

package engine

import "github.com/jczastkiewicz/crucible/pkg/collect"

// ZoneType is where a card is. Ported from
// forge-game/src/main/java/forge/game/zone/ZoneType.java, in its declaration
// order, because card scripts name these by string and the order is what a
// `ZoneType.listValueOf` comparison sees.
type ZoneType uint8

// The zones, in Java's declaration order.
const (
	Hand ZoneType = iota
	Library
	Graveyard
	Battlefield
	Exile
	Flashback
	Command
	Stack
	Sideboard
	Ante
	Merged
	SchemeDeck
	PlanarDeck
	AttractionDeck
	Junkyard
	ContraptionDeck
	Subgame
	ExtraHand
	None

	numZoneTypes = int(None) + 1
)

var zoneNames = [numZoneTypes]string{
	Hand: "Hand", Library: "Library", Graveyard: "Graveyard", Battlefield: "Battlefield",
	Exile: "Exile", Flashback: "Flashback", Command: "Command", Stack: "Stack",
	Sideboard: "Sideboard", Ante: "Ante", Merged: "Merged", SchemeDeck: "SchemeDeck",
	PlanarDeck: "PlanarDeck", AttractionDeck: "AttractionDeck", Junkyard: "Junkyard",
	ContraptionDeck: "ContraptionDeck", Subgame: "Subgame", ExtraHand: "ExtraHand",
	None: "None",
}

// String returns the zone as a card script spells it.
func (z ZoneType) String() string {
	if int(z) >= numZoneTypes {
		return "None"
	}
	return zoneNames[z]
}

// ZoneByName looks a zone up by the name a script writes, and reports whether
// it is one. Matching is exact: Java's ZoneType.smartValueOf is case-sensitive
// after its own trim, and a zone that silently resolves to None is a card that
// silently does nothing.
func ZoneByName(name string) (ZoneType, bool) {
	for i, n := range zoneNames {
		if n == name {
			return ZoneType(i), true
		}
	}
	return None, false
}

// Zone is one player's copy of one zone, or the single shared copy for the
// zones that have no owner.
//
// Order is load-bearing everywhere: a library is a stack, a graveyard's order
// decides `CardsInGraveyard` counts and delirium, and the battlefield's order
// is the tiebreak Java uses when two triggers are otherwise simultaneous
// (GO-12).
type Zone struct {
	Type  ZoneType
	Owner PlayerID
	cards *collect.OrderedSet[CardID]
}

// Cards returns the zone's contents in order. The slice is the zone's own, so
// a caller that needs to hold it past the next mutation copies it.
func (z *Zone) Cards() []CardID { return z.cards.All() }

// Len is how many cards the zone holds.
func (z *Zone) Len() int { return z.cards.Len() }

// Contains reports whether the card is in this zone.
func (z *Zone) Contains(id CardID) bool { return z.cards.Contains(id) }
