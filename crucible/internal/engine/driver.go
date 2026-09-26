// The turn driver: whole steps, turns and games played through real
// priority rounds. ADR-0026.
//
// Ported from PhaseHandler.mainGameLoop/mainLoopStep
// (forge-game/src/main/java/forge/game/phase/PhaseHandler.java:1032-1160):
// each step's turn-based actions and phase triggers (beginStep, turn.go),
// then a priority round if the step grants one (priorityRound,
// priority.go), then the next step. AdvancePhase stays the bookkeeping walk
// fixtures use; only this file drives a step.

package engine

import "errors"

// errRunBeforeStartTurn is Run's own GO-7 stop: a game with no turn begun
// has no active player or step to drive.
var errRunBeforeStartTurn = errors.New("engine: Run called before StartTurn")

// Step leaves the current step and plays out the next one: its turn-based
// actions -- combat's declarations and damage included -- and phase
// triggers (beginStep, driven), then a priority round if the step grants
// one. A Cleanup that grants priority is followed by another Cleanup in the
// same call (CR 514.3a, PhaseHandler.java:156-158), until one grants none.
//
// The current step's own priority window is taken as already played: StartTurn
// begins Untap, which grants none, so StartTurn followed by Step or Run
// plays a whole game.
//
// A Cleanup that EndTurn begins during resolution (endturneffect.go) is not
// repeated: its state-based result is discarded, ADR-0026 Decision 5's named
// gap. Only a Cleanup this call began itself repeats.
func (g *Game) Step(reg *Registry, controller PlayerController) error {
	if g.over {
		return nil
	}
	priority, err := g.advanceStep(controller, true)
	for {
		if err != nil {
			return err
		}
		if err := g.TakePendingError(); err != nil {
			return err
		}
		if g.over || !priority {
			return nil
		}
		began := g.activePhase
		if err := g.priorityRound(reg, controller); err != nil {
			return err
		}
		if g.over || began != Cleanup || g.activePhase != Cleanup {
			return nil
		}
		priority, err = g.beginStep(controller, true)
	}
}

// Run steps until the game is over or turn maxTurns's Cleanup has
// finished, checked at the Cleanup-to-Untap boundary so a capped game
// always stops after a whole turn (the P7 gate's turn cap). A capped game
// returns nil with Over() still false: whether that is a draw is the
// caller's decision (M8), not a rules outcome. Java has no cap
// (PhaseHandler.java:1032-1037).
//
// There is no cap on actions within one priority round: ScriptedController
// is finite, and a controller that never passes is M7's to guard against,
// where Java put its own (PhaseHandler.java:1102-1105).
func (g *Game) Run(reg *Registry, controller PlayerController, maxTurns int) error {
	if g.turn == 0 {
		return errRunBeforeStartTurn
	}
	for !g.over {
		if g.activePhase == Cleanup && g.turn >= maxTurns {
			return nil
		}
		if err := g.Step(reg, controller); err != nil {
			return err
		}
	}
	return nil
}
