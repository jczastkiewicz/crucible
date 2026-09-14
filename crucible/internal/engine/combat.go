// Combat's own state: CR 506-510, starting with CR 508's attackers, the
// only piece this port has reached (attack.go).

package engine

// Combat is the game's current combat, if one is happening. Attackers is who
// was declared this combat (CR 508.1); Blocks is who blocks them (CR 509.1).
// Damage assignment and the rest of CR 508-510 land once something needs
// them (M5-M6).
type Combat struct {
	Attackers []CardID
	Blocks    []Block
}

// Block is one blocking assignment: Blocker blocks Attacker (CR 509.1). A
// single Attacker can appear in more than one Block -- gang blocking is
// ordinary, CR 509.1c only limits how many attackers one blocker can
// choose, not the reverse.
type Block struct {
	Blocker, Attacker CardID
}

// clone is Combat's half of Game.Clone: a shared backing array would let a
// declaration on the clone alias the original, the same reasoning PT's own
// clone has.
func (c Combat) clone() Combat {
	return Combat{
		Attackers: append([]CardID(nil), c.Attackers...),
		Blocks:    append([]Block(nil), c.Blocks...),
	}
}
