// Turn structure.

package engine

import "strings"

// PhaseType is a step or phase of a turn. Ported from
// forge-game/src/main/java/forge/game/phase/PhaseType.java in its declaration
// order, because card scripts write `Phase$ Upkeep` and compare against it.
type PhaseType uint8

// The phases and steps, in turn order.
const (
	Untap PhaseType = iota
	Upkeep
	Draw
	Main1
	CombatBegin
	DeclareAttackers
	DeclareBlockers
	FirstStrikeDamage
	CombatDamage
	CombatEnd
	Main2
	EndOfTurn
	Cleanup

	numPhaseTypes = int(Cleanup) + 1
)

// phaseNames are the names card scripts write, which are Java's second
// constructor argument rather than the constant. `Phase$ End of Turn` carries
// its spaces, so these are not identifiers.
var phaseNames = [numPhaseTypes]string{
	Untap: "Untap", Upkeep: "Upkeep", Draw: "Draw", Main1: "Main1",
	CombatBegin: "BeginCombat", DeclareAttackers: "Declare Attackers",
	DeclareBlockers: "Declare Blockers", FirstStrikeDamage: "First Strike Damage",
	CombatDamage: "Combat Damage", CombatEnd: "EndCombat", Main2: "Main2",
	EndOfTurn: "End of Turn", Cleanup: "Cleanup",
}

// String returns the phase as a card script spells it.
func (p PhaseType) String() string {
	if int(p) >= numPhaseTypes {
		return "Cleanup"
	}
	return phaseNames[p]
}

// PhaseByName looks a phase up by the name a script writes, and reports
// whether it is one.
func PhaseByName(name string) (PhaseType, bool) {
	for i, n := range phaseNames {
		if n == name {
			return PhaseType(i), true
		}
	}
	return Untap, false
}

// IsCombat reports whether the phase is part of combat, which several triggers
// and continuous effects restrict themselves to.
func (p PhaseType) IsCombat() bool {
	return p >= CombatBegin && p <= CombatEnd
}

// phaseSet is a set of phases, one bit per PhaseType.
type phaseSet uint16

func (s phaseSet) has(p PhaseType) bool { return s&(1<<uint(p)) != 0 }

// parsePhaseRange is PhaseType.parseRange: a comma list of phase names
// (case-insensitive, trimmed -- smartValueOf), "Main" for both main
// phases, and "A->B" (B blank meaning Cleanup) for the inclusive range. ok
// is false for any token that names no phase.
func parsePhaseRange(values string) (phaseSet, bool) {
	var set phaseSet
	for _, raw := range strings.Split(values, ",") {
		tok := strings.TrimSpace(raw)
		if from, to, isRange := strings.Cut(tok, "->"); isRange {
			f, ok := phaseNameFold(strings.TrimSpace(from))
			if !ok {
				return 0, false
			}
			t := Cleanup
			if strings.TrimSpace(to) != "" {
				if t, ok = phaseNameFold(strings.TrimSpace(to)); !ok {
					return 0, false
				}
			}
			for p := f; p <= t; p++ {
				set |= 1 << uint(p)
			}
			continue
		}
		if strings.EqualFold(tok, "Main") {
			set |= 1<<uint(Main1) | 1<<uint(Main2)
			continue
		}
		p, ok := phaseNameFold(tok)
		if !ok {
			return 0, false
		}
		set |= 1 << uint(p)
	}
	return set, true
}

// phaseNameFold is PhaseByName's own case-insensitive twin, needed only
// here: the corpus itself is inconsistent about one phase's own case
// (`Phase$ End of Turn`, 677 real lines; `Phase$ End Of Turn`, 3 more, a
// capital `O`) the way ZoneByName's own doc comment says zone names never
// are, so an exact match would silently drop those 3 real lines rather than
// fire their trigger. PhaseByName itself stays exact -- fixture.go's own
// GameState text format is this port's own, not the corpus's, and has no
// such inconsistency to tolerate.
func phaseNameFold(name string) (PhaseType, bool) {
	for i, n := range phaseNames {
		if strings.EqualFold(n, name) {
			return PhaseType(i), true
		}
	}
	return Untap, false
}
