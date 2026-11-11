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
	// lethal this turn (Card.hasBeenDealtExcessDamageThisTurn, CR 120.10):
	// the wasDealtExcessDamageThisTurn valid property reads it. Set as the
	// damage is marked (dealPermanentDamage), so a DamageDone trigger of the
	// same batch already sees it.
	ExcessThisTurn bool
	// ExcessReceived is the excess damage the card was dealt this turn, summed
	// over every damage batch (Card.logExcessDamage, Card.java:6193-6195):
	// Count$ExcessDamageReceivedThisTurn reads it.
	ExcessReceived int
	// Sources are the cards that dealt damage to this one this turn, in
	// order (Card.getDamageReceivedThisTurn's source half): the "DamagedBy"
	// valid property reads them.
	Sources []CardID
	// bySource is the marked damage per source (Card.damage, merged per
	// source): getMaxDamageFromSource (Card.java:6201-6203) reads its largest,
	// for Ogre Enforcer. A heal or the cleanup step clears it with Marked.
	bySource []sourceDamage
}

// sourceDamage is one source's share of the marked damage.
type sourceDamage struct {
	source CardID
	amount int
}

// MaxFromSource is Card.getMaxDamageFromSource: the most marked damage any
// one source dealt; 0 for none.
func (d *Damage) MaxFromSource() int {
	best := 0
	for _, s := range d.bySource {
		best = max(best, s.amount)
	}
	return best
}

// Mark adds damage from no known source (a fixture's Damage:N). A deathtouch
// source sets the flag for the rest of the turn, so a later non-deathtouch
// point cannot clear it.
func (d *Damage) Mark(amount int, deathtouch bool) { d.MarkFrom(NoCard, amount, deathtouch) }

// MarkFrom is Mark for damage source dealt, kept per source as well.
func (d *Damage) MarkFrom(source CardID, amount int, deathtouch bool) {
	if amount <= 0 {
		return
	}
	d.Marked += amount
	merged := false
	for i := range d.bySource {
		if d.bySource[i].source == source {
			d.bySource[i].amount += amount
			merged = true
			break
		}
	}
	if !merged {
		d.bySource = append(d.bySource, sourceDamage{source: source, amount: amount})
	}
	if deathtouch {
		d.Deathtouch = true
	}
}

// Clear wipes marked damage, which is what the cleanup step does. The
// deathtouch and excess flags go with it, because both are scoped to the turn.
func (d *Damage) Clear() { *d = Damage{} }

// Heal is Clear for a heal or a regeneration (Card.healDamage,
// Card.java:6181-6185): the excess flag and total stay for the rest of the
// turn, since only the cleanup step resets them (Card.resetExcessDamage,
// :6202-6205, called at :7285), and so do the sources: healDamage clears the marked damage and
// the assigned damage map, never damageReceivedThisTurn (GameEntity.java:56),
// which DamagedBy and wasDealtDamageThisTurn read.
func (d *Damage) Heal() {
	*d = Damage{ExcessThisTurn: d.ExcessThisTurn, ExcessReceived: d.ExcessReceived, Sources: d.Sources}
}
