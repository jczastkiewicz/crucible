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

// TypeEffect is one continuous effect's own AddType$/RemoveType$
// contribution. AddTypes is unioned into the running type line, then
// RemoveTypes is subtracted from it, in that order within one effect --
// applyContinuousType's only caller never builds one carrying an overlapping
// add and remove, so the order between the two never matters in practice.
type TypeEffect struct {
	Timestamp             uint64
	AddTypes, RemoveTypes cardtype.Line
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

// foldType applies every TypeEffect to base in Timestamp order (CR 613.7):
// each effect's own AddTypes is unioned in, then its own RemoveTypes is
// subtracted, before the next effect (by timestamp) runs against the
// result -- CR 613.8's dependency reordering is not in play, foldPT's own
// doc comment gives the identical reason (nothing here has more than one
// continuous effect on the same card yet to depend on another).
func foldType(base cardtype.Line, effects []TypeEffect) cardtype.Line {
	sorted := append([]TypeEffect(nil), effects...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Timestamp < sorted[j].Timestamp })
	result := base
	for _, e := range sorted {
		result = result.Union(e.AddTypes).Without(e.RemoveTypes)
	}
	return result
}
