// Delayed and reflexive triggers: triggered abilities an effect creates
// rather than a card's own T: line (CR 603.7, 603.12).
//
// Ported from forge-game/src/main/java/forge/game/trigger/
// TriggerHandler.java's delayed-trigger lists (registerDelayedTrigger,
// registerThisTurnDelayedTrigger, registerPlayerDefinedDelayedTrigger,
// clearThisTurnDelayedTrigger, handlePlayerDefinedDelTriggers) and the
// delayed half of runWaitingTrigger.

package engine

import (
	"strings"

	"github.com/jczastkiewicz/crucible/internal/carddb/compile"
	"github.com/jczastkiewicz/crucible/internal/expr"
)

// delayedTrigger is one registered delayed trigger: the DelayedTrigger
// line itself (Mode$, Phase$, ValidPlayer$, Execute$), who created it and
// what it remembered. A delayed trigger fires once and is gone.
type delayedTrigger struct {
	Trigger    *compile.Ability
	Host       CardID
	Controller PlayerID
	Amounts    map[string]expr.Amount
	Remembered []EntityID
	// ThisTurn drops the trigger when the next turn begins
	// (clearThisTurnDelayedTrigger) if it has not fired.
	ThisTurn bool
	// AtCleanup holds the trigger inactive until this turn's cleanup
	// (NextTurn$/UpcomingTurn$, Java's getCleanup().addUntil).
	AtCleanup bool
	// ForPlayer holds the trigger inactive until that player's next turn
	// begins (DelayedTriggerDefinedPlayer$).
	ForPlayer PlayerID
}

func (d *delayedTrigger) active() bool { return !d.AtCleanup && d.ForPlayer == NoPlayer }

// activateCleanupDelayedTriggers is the cleanup-time registration of
// NextTurn$/UpcomingTurn$ triggers. Java runs it from the cleanup phase's
// end, after handleNextTurn has already cleared the ending turn's ThisTurn
// triggers, so a NextTurn$ trigger is live for exactly the turn that
// follows -- AdvancePhase calls it right after delayedTriggersOnNextTurn.
func (g *Game) activateCleanupDelayedTriggers() {
	for i := range g.delayed {
		g.delayed[i].AtCleanup = false
	}
}

// delayedTriggersOnNextTurn is handleNextTurn's delayed-trigger half:
// ThisTurn triggers that never fired are dropped, then triggers waiting on
// the incoming active player's turn become live.
func (g *Game) delayedTriggersOnNextTurn(next PlayerID) {
	kept := g.delayed[:0]
	for _, d := range g.delayed {
		if d.ThisTurn && d.active() {
			continue
		}
		if d.ForPlayer == next {
			d.ForPlayer = NoPlayer
		}
		kept = append(kept, d)
	}
	g.delayed = kept
}

// delayedPhaseTriggerMatches collects every live delayed Mode$ Phase
// trigger the current step satisfies, removing each one it collects --
// runWaitingTrigger's `delayedTriggers.remove(deltrig)` before
// runSingleTrigger. A spawned trigger has no zone check (TriggerHandler
// skips zonesCheck when getSpawningAbility() != null), and one whose
// controller has left the game is dropped.
func (g *Game) delayedPhaseTriggerMatches() []Ability {
	var matches []Ability
	kept := g.delayed[:0]
	for _, d := range g.delayed {
		if g.Player(d.Controller).Lost {
			continue
		}
		mode, _ := d.Trigger.Param("Mode")
		if !d.active() || !strings.EqualFold(mode, "Phase") || !phaseTriggerMatches(d.Trigger, "Phase", g.activePhase) {
			kept = append(kept, d)
			continue
		}
		if validPlayer, ok := d.Trigger.Param("ValidPlayer"); ok {
			matched, recognized := matchesPlayerSpec(g, g.activePlayer, d.Controller, d.Host, validPlayer)
			if !recognized || !matched {
				kept = append(kept, d)
				continue
			}
		}
		sub, api, optional, ok := triggerEffectAPI(g, g.Card(d.Host), d.Amounts, d.Trigger)
		if !ok {
			kept = append(kept, d)
			continue
		}
		matches = append(matches, Ability{
			API: api, Source: d.Host, Controller: d.Controller, Params: sub,
			Amounts: d.Amounts, Optional: optional, TriggerRemembered: d.Remembered,
		})
	}
	g.delayed = kept
	return matches
}
