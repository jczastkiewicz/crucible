// ControlMod: Layer 2's own control-changing continuous effects, the
// mutable part Card.Controller (card.go) folds against the card's own base
// controller.

package engine

// ControlMod is the Layer 2 continuous effects currently affecting one
// card -- CR 613.1's own control layer, GainControl$ (StaticAbilityContinuous
// .java's own CONTROL branch, applyOneContinuousControl's doc comment in
// continuous.go has the full corpus breakdown: 44 real S:Mode$ Continuous
// lines, 43 of them the plain "GainControl$ You" shape this resolves).
type ControlMod struct {
	effects []ControlEffect
}

// ControlEffect is one continuous effect's claim to control this card.
// Timestamp orders it against every other claim on the same card --
// Card.Controller's own fold picks the highest-Timestamp entry
// unconditionally, Java's own Card.getController() (tempControllers, a
// NavigableMap<Long, Player>) with the "lastTimestamp > controllerTimestamp"
// guard collapsed away: that guard exists only to let Java's own
// setController (an explicit "gain control permanently" one-shot effect,
// game-state.md's "Not ported yet") outrank an OLDER temp-controller, a
// mechanism this port does not build yet, so there is no base-controller
// timestamp here for a temp-controller to ever lose to.
type ControlEffect struct {
	Timestamp  uint64
	Controller PlayerID
}

// Add records one continuous effect. Order does not matter here for the
// same reason PT.Add's own doc comment gives: Controller picks the
// highest-Timestamp entry itself rather than relying on append order.
func (m *ControlMod) Add(e ControlEffect) { m.effects = append(m.effects, e) }

// Clear removes every effect -- applyContinuousControl's own recompute-fresh
// pass (continuous.go), PT.Clear's own reasoning applied to Layer 2.
func (m *ControlMod) Clear() { m.effects = nil }

// clone is ControlMod's half of Game.Clone, PT.clone's own reasoning: a
// shared backing array would let a push on the clone alias the original.
func (m ControlMod) clone() ControlMod {
	return ControlMod{effects: append([]ControlEffect(nil), m.effects...)}
}
