package engine

import "github.com/jczastkiewicz/crucible/internal/carddb/compile"

// phaseGroups is PhaseType.PHASE_GROUPS: the steps inside a group always run
// forward, only the order of the groups reverses under Topsy Turvy.
var phaseGroups = [][]PhaseType{
	{Untap, Upkeep, Draw},
	{Main1},
	{CombatBegin, DeclareAttackers, DeclareBlockers, FirstStrikeDamage, CombatDamage, CombatEnd},
	{Main2},
	{EndOfTurn},
	{Cleanup},
}

// nextPhaseOf is PhaseType.getNext (PhaseType.java:142-158): the step after
// cur, walking the groups backwards when reversed. Cleanup stays the turn's
// last step either way: it is the only group-5 step and its successor group is
// the one before it (End step reversed, Untap forward).
func nextPhaseOf(cur PhaseType, reversed bool) PhaseType {
	gi := 0
	for i, grp := range phaseGroups {
		for j, ph := range grp {
			if ph != cur {
				continue
			}
			if j+1 < len(grp) {
				return grp[j+1]
			}
			if reversed {
				gi = (i - 1 + len(phaseGroups)) % len(phaseGroups)
			} else {
				gi = (i + 1) % len(phaseGroups)
			}
			return phaseGroups[gi][0]
		}
	}
	return Untap
}

// phasesReversed is Player.isPhasesReversed (StaticAbilityTurnPhaseReversed):
// each PhaseReversed static whose ValidPlayer$ matches toggles the answer, so
// two sources cancel.
func (g *Game) phasesReversed(pid PlayerID) bool {
	return g.reversalToggles(pid, "PhaseReversed")
}

// turnReversedByStatic is Player.isTurnOrderReversed: the same parity over
// TurnReversed statics, applied on top of the game's own direction
// (Game.getTurnOrder, Game.java:551-556).
func (g *Game) turnReversedByStatic(pid PlayerID) bool {
	return g.reversalToggles(pid, "TurnReversed")
}

// turnOrderIsReversed is whether turn order currently runs backwards: the
// ReverseTurnOrder flag flipped by the active player's TurnReversed statics.
func (g *Game) turnOrderIsReversed() bool {
	return g.turnOrderReversed != g.turnReversedByStatic(g.activePlayer)
}

func (g *Game) reversalToggles(pid PlayerID, mode string) bool {
	odd := false
	g.playerStatic(pid, mode, func(*compile.Ability) bool {
		odd = !odd
		return false
	})
	return odd
}
