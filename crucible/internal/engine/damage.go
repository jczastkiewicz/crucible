// Damage marked on a card.

package engine

// Damage is what a creature has been dealt this turn.
//
// Marked damage is not a counter and does not use the counter machinery: it
// clears in the cleanup step rather than persisting, and it is compared
// against toughness rather than added to it.
type Damage struct {
	// Marked is the damage currently on the card.
	Marked int
	// Deathtouch records that some of it came from a deathtouch source, which
	// is what the state-based action checks rather than the amount.
	Deathtouch bool
	// ExcessThisTurn records that the card was dealt more damage than was
	// lethal, which several cards care about.
	ExcessThisTurn bool
}

// Mark adds damage. A deathtouch source sets the flag for the rest of the
// turn, so a later non-deathtouch point cannot clear it.
func (d *Damage) Mark(amount int, deathtouch bool) {
	if amount <= 0 {
		return
	}
	d.Marked += amount
	if deathtouch {
		d.Deathtouch = true
	}
}

// Clear wipes marked damage, which is what the cleanup step does. The
// deathtouch and excess flags go with it, because both are scoped to the turn.
func (d *Damage) Clear() { *d = Damage{} }
