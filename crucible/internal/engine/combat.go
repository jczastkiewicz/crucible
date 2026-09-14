// Combat's own state: CR 506-510, starting with CR 508's attackers, the
// only piece this port has reached (attack.go, block.go).

package engine

// Combat is the game's current combat, if one is happening. Attackers is who
// was declared this combat (CR 508.1); AttackTargets is what each one is
// attacking -- a player, or a planeswalker/battle that player controls (CR
// 508.1d); Blocks is who blocks them (CR 509.1). The rest of CR 508-510
// lands once something needs it (M5-M6).
type Combat struct {
	Attackers     []CardID
	AttackTargets map[CardID]EntityID
	Blocks        []Block
}

// Block is one blocking assignment: Blocker blocks Attacker (CR 509.1). A
// single Attacker can appear in more than one Block -- gang blocking is
// ordinary, CR 509.1c only limits how many attackers one blocker can
// choose, not the reverse.
type Block struct {
	Blocker, Attacker CardID
}

// DamageAssignment is one entry in the order an attacking player divides a
// gang-blocked attacker's combat damage (CR 510.1c, combatdamage.go):
// Blocker receives Amount, and entries are applied in the order returned.
// The rule requires each blocker in that order to receive at least its
// lethal amount before any is left for the next one, but that isn't checked
// here -- trust the controller's answer, the same as ChooseLegendaryToKeep.
type DamageAssignment struct {
	Blocker CardID
	Amount  int
}

// clone is Combat's half of Game.Clone: a shared backing array would let a
// declaration on the clone alias the original, the same reasoning PT's own
// clone has.
func (c Combat) clone() Combat {
	var targets map[CardID]EntityID
	if c.AttackTargets != nil {
		targets = make(map[CardID]EntityID, len(c.AttackTargets))
		for k, v := range c.AttackTargets {
			targets[k] = v
		}
	}
	return Combat{
		Attackers:     append([]CardID(nil), c.Attackers...),
		AttackTargets: targets,
		Blocks:        append([]Block(nil), c.Blocks...),
	}
}
