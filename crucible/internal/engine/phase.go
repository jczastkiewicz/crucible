// Turn structure.

package engine

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
