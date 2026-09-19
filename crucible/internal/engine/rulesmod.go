// RulesMod: Layer 8's own player-facing continuous effects, the mutable
// part a Player's own HandSizeLimit/LandPlayLimit (player.go) folds against
// the printed defaults (MaxHandSize, land.go's own default land-play count).

package engine

// RulesMod is the Layer 8 continuous effects currently affecting one
// player -- CR 613's own rule-changing catch-all layer, which carries no CR
// number of its own (layer.go's own doc comment), trimmed to the corpus's
// two dominant real shapes: SetMaxHandSize$/RaiseMaxHandSize$ (a player's
// own maximum hand size, 43 and 8 real lines) and AdjustLandPlays$ (how many
// lands a turn allows, 27 real lines). Every other Layer 8 param
// (MayLookAt$, 88; MayPlay$, 181; vote/villainous-choice params, a handful
// each) needs its own separate mechanic this port does not build --
// applyOneContinuousRules's own doc comment (continuous.go) has the reasons.
type RulesMod struct {
	effects []RulesEffect
}

// RulesEffect is one continuous effect's contribution to a player's own
// hand-size and/or land-play limit. HasSetHandSize/HasRaiseHandSize mirror
// PTEffect's own HasPower/HasToughness split (pt.go): a real corpus line
// carries at most one of SetMaxHandSize$/RaiseMaxHandSize$, never both, but
// the flags still let HandSizeLimit (player.go) apply only the dimension a
// given effect actually names, the same reason PTEffect's own flags exist.
//
// HasAdjustLandPlays plays the identical role for AdjustLandPlays$, even
// though Player.getMaxLandPlays's own unconditional sum (Java) would make a
// bare zero-value RulesEffect harmless there too (0 added is already a
// no-op) -- kept anyway so a RulesEffect that names only a hand-size
// dimension cannot be misread as also asserting "adjust land plays by 0."
type RulesEffect struct {
	// Timestamp orders this effect against every other one on the same
	// player, foldPT's own combine convention (card.go) -- the one dimension
	// here that is order-dependent (HandSizeLimit's own doc comment).
	Timestamp uint64

	HasSetHandSize       bool
	SetHandSize          int
	SetHandSizeUnlimited bool

	HasRaiseHandSize bool
	RaiseHandSize    int

	HasAdjustLandPlays       bool
	AdjustLandPlays          int
	AdjustLandPlaysUnlimited bool
}

// Add records one continuous effect. Order does not matter here for the
// same reason PT.Add's own doc comment gives: HandSizeLimit sorts by
// Timestamp itself before folding.
func (r *RulesMod) Add(e RulesEffect) { r.effects = append(r.effects, e) }

// Clear removes every effect -- applyContinuousRules' own recompute-fresh
// pass (continuous.go), PT.Clear's own reasoning applied to a player rather
// than a card.
func (r *RulesMod) Clear() { r.effects = nil }

// clone is RulesMod's half of Game.Clone, PT.clone's own reasoning: a
// shared backing array would let a push on the clone alias the original.
func (r RulesMod) clone() RulesMod {
	return RulesMod{effects: append([]RulesEffect(nil), r.effects...)}
}
