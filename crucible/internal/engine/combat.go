// Combat's own state: CR 506-510, starting with CR 508's attackers, the
// only piece this port has reached (attack.go).

package engine

// Combat is the game's current combat, if one is happening. Attackers is
// who was declared this combat (CR 508.1); blockers, damage assignment and
// the rest of CR 508-510 land once something needs them (M5-M6).
type Combat struct {
	Attackers []CardID
}

// clone is Combat's half of Game.Clone: a shared backing array would let a
// declaration on the clone alias the original, the same reasoning PT's own
// clone has.
func (c Combat) clone() Combat {
	return Combat{Attackers: append([]CardID(nil), c.Attackers...)}
}
