// KeywordMod: Layer 6's own continuous ability-granting effects, the
// keyword counterpart to TypeMod's/ColorMod's own type-line/color layers.

package engine

// KeywordMod is the continuous keyword grants currently affecting one card
// -- CR 613.4's Layer 6. Unlike TypeMod/ColorMod, there is no fold order to
// resolve: HasKeyword (card.go) only ever asks "is this keyword present,"
// never "what is the current value," so every effect's own AddKeywords
// simply contributes to the same membership test regardless of Timestamp --
// CR 613.7's tiebreak only matters where two effects could disagree about
// the SAME thing (Layer 7's Set-vs-Modify, Layer 5's Set-vs-Add), and two
// continuous effects both granting a keyword never disagree about anything.
type KeywordMod struct {
	effects []KeywordEffect
}

// KeywordEffect is one continuous effect's own AddKeyword$ contribution:
// every keyword line it grants, verbatim -- exactly the form a real K: line
// would carry ("Ward:2", "First Strike", "Protection:..."), keyword.Parse's
// own job to split further at HasKeyword's own query time, the identical
// treatment a printed keyword line already gets. Removing a keyword
// continuously (RemoveKeyword$/RemoveAllAbilities$) is not represented
// here: applyOneContinuousKeyword (continuous.go) skips a whole line that
// carries either rather than resolving only the add half of a "gains X,
// loses Y" line, so nothing yet needs a Remove side to fold against.
type KeywordEffect struct {
	Timestamp   uint64
	AddKeywords []string
}

// Add records one continuous effect. Order does not matter here, the same
// as it never has for PT.Add/TypeMod.Add/ColorMod.Add -- folding (or, for
// this one, plain membership) does not depend on insertion order.
func (km *KeywordMod) Add(e KeywordEffect) { km.effects = append(km.effects, e) }

// Clear removes every effect, which Move calls when a card leaves the
// battlefield -- PT.Clear's own reason applies identically here.
func (km *KeywordMod) Clear() { km.effects = nil }

// clone is KeywordMod's half of Game.Clone -- PT.clone's own reasoning: a
// shared backing array would let a push on the clone alias the original.
func (km KeywordMod) clone() KeywordMod {
	return KeywordMod{effects: append([]KeywordEffect(nil), km.effects...)}
}
