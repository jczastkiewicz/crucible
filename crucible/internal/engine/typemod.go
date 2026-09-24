// TypeMod: Layer 4's own continuous type-changing effects, the type-line
// counterpart to PT's own power/toughness layer (pt.go).

package engine

import (
	"sort"

	"github.com/jczastkiewicz/crucible/internal/cardtype"
)

// TypeMod is the continuous type-changing effects currently affecting one
// card -- CR 613.4, applied in Timestamp order (CR 613.7's own tiebreak,
// the same one foldPT (card.go) already uses for Layer 7).
type TypeMod struct {
	effects []TypeEffect
}

// TypeEffect is one continuous effect's own type change, applied in
// CardChangedType.applyChanges' order: the whole-category removals first
// (RemoveCardTypes keeps Instant/Sorcery, CR 205.1a; RemoveSubTypes wins over
// DropSubtype, the per-category RemoveCreatureTypes$/RemoveLandTypes$/...
// test), then RemoveTypes is subtracted, then AddTypes is unioned in -- so
// "becomes an artifact creature" (RemoveCardTypes$ plus Types$) keeps what it
// adds.
type TypeEffect struct {
	Timestamp             uint64
	AddTypes, RemoveTypes cardtype.Line
	RemoveCardTypes       bool
	RemoveSuperTypes      bool
	RemoveSubTypes        bool
	DropSubtype           func(string) bool
}

// Add records one continuous effect. Order does not matter here: folding
// sorts by Timestamp, not insertion order -- foldPT's own contract.
func (tm *TypeMod) Add(e TypeEffect) { tm.effects = append(tm.effects, e) }

// Clear removes every effect, which Move calls when a card leaves the
// battlefield -- PT.Clear's own reason applies identically here: a
// type-changing effect that only applied there stops the moment it does,
// and this port has no duration tracking for one that would need to
// outlast the leaving (game-state.md's "Not ported yet").
func (tm *TypeMod) Clear() { tm.effects = nil }

// clone is TypeMod's half of Game.Clone -- PT.clone's own reasoning: a
// shared backing array would let a push on the clone alias the original.
func (tm TypeMod) clone() TypeMod {
	return TypeMod{effects: append([]TypeEffect(nil), tm.effects...)}
}

// foldType applies every TypeEffect to base in Timestamp order (CR 613.7),
// each in TypeEffect's own removal-then-addition order, before the next
// effect (by timestamp) runs against the result -- CR 613.8's dependency
// reordering is not in play, foldPT's own doc comment gives the identical
// reason.
func foldType(base cardtype.Line, effects []TypeEffect) cardtype.Line {
	sorted := append([]TypeEffect(nil), effects...)
	sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].Timestamp < sorted[j].Timestamp })
	result := base
	for _, e := range sorted {
		if e.RemoveCardTypes {
			result = result.WithoutCardTypes()
		}
		if e.RemoveSuperTypes {
			result = result.WithoutSupertypes()
		}
		if e.RemoveSubTypes {
			result = result.WithoutSubtypes()
		} else if e.DropSubtype != nil {
			result = result.WithoutSubtypesWhere(e.DropSubtype)
		}
		result = result.Without(e.RemoveTypes).Union(e.AddTypes)
	}
	return result
}
