// ColorMod: Layer 5's own continuous color-changing effects, the color
// counterpart to TypeMod's own type-line layer (typemod.go).

package engine

import (
	"sort"

	"github.com/jczastkiewicz/crucible/internal/mana"
)

// ColorMod is the continuous color-changing effects currently affecting one
// card -- CR 613.4, applied in Timestamp order, foldPT's own tiebreak
// (card.go) reused identically here.
type ColorMod struct {
	effects []ColorEffect
}

// ColorEffect is one continuous effect's own AddColor$/SetColor$
// contribution. Overwrite is SetColor$'s own "replace the running color set
// outright" (Java's overwriteColors, StaticAbilityContinuous.java); false is
// AddColor$'s own "union in," the same Add/Set split Layer 7's own
// PTEffect.Layer already draws between LayerSetPT and LayerModifyPT, just
// collapsed to one bool since Layer 5 has no third sub-layer to distinguish.
type ColorEffect struct {
	Timestamp uint64
	Colors    mana.Colors
	Overwrite bool
}

// Add records one continuous effect. Order does not matter here: folding
// sorts by Timestamp, not insertion order -- foldPT's own contract.
func (cm *ColorMod) Add(e ColorEffect) { cm.effects = append(cm.effects, e) }

// Clear removes every effect, which Move calls when a card leaves the
// battlefield -- PT.Clear's own reason applies identically here.
func (cm *ColorMod) Clear() { cm.effects = nil }

// clone is ColorMod's half of Game.Clone -- PT.clone's own reasoning: a
// shared backing array would let a push on the clone alias the original.
func (cm ColorMod) clone() ColorMod {
	return ColorMod{effects: append([]ColorEffect(nil), cm.effects...)}
}

// foldColor applies every ColorEffect to base in Timestamp order (CR
// 613.7): an Overwrite effect replaces the running color set outright, a
// non-Overwrite one unions its own Colors into it -- foldType's own doc
// comment gives the identical reason CR 613.8's dependency reordering is
// not in play (nothing here has more than one continuous effect on the
// same card yet to depend on another).
func foldColor(base mana.Colors, effects []ColorEffect) mana.Colors {
	sorted := append([]ColorEffect(nil), effects...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Timestamp < sorted[j].Timestamp })
	result := base
	for _, e := range sorted {
		if e.Overwrite {
			result = e.Colors
		} else {
			result |= e.Colors
		}
	}
	return result
}
