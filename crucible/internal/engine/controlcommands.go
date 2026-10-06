// Card and phase commands: Java's GameCommand lists, as data.
//
// Ported from forge-game/src/main/java/forge/game/card/Card.java:3562-3640
// (add*Command / run*Commands), SpellAbilityEffect.java:949-1040
// (addUntilCommand, checkValidDuration), ControlGainEffect.java:137-215
// (getLoseControlCommand and its registrations) and GameAction.java:994-1033
// (controllerChangeZoneCorrection's runChangeControllerCommands).
//
// Java registers anonymous closures. A closure would be code in the game
// state, so each one this port needs is a cardCommand record naming what it
// does and to which card (ADR-0030's "data only" shape, scheduledaction.go):
// two kinds exist, ControlGainEffect's lose-control command and an Effect
// card's "this effect ends" command.

package engine

//enginelint:allow id card game zone player control

import (
	"fmt"
	"strings"
)

// cardCommandKind is which Java closure a cardCommand stands for.
type cardCommandKind uint8

const (
	// commandLoseControl is ControlGainEffect.getLoseControlCommand: the
	// control change made at Timestamp on Target ends.
	commandLoseControl cardCommandKind = iota
	// commandEndEffect is addUntilCommand's closure for an Effect card: the
	// effect card Target leaves the Command zone.
	commandEndEffect
)

// cardCommand is one pending GameCommand. Host is the card whose command
// list or phase list it sits on (ControlGainEffect's source).
type cardCommand struct {
	Kind      cardCommandKind
	Target    CardID
	Timestamp uint64
	Host      CardID
}

// playerCommand is a command that waits for a player's next turn to end
// (Phase.addUntilEnd/registerUntilEnd): Armed marks one registered during
// that player's own turn, which survives that turn's cleanup
// (registerUntilEndCommand).
type playerCommand struct {
	cardCommand
	Player PlayerID
	Armed  bool
}

// removeTempController is Card.removeTempController(long): the control
// change made at ts ends. It reports whether there was one.
func (c *Card) removeTempController(ts uint64) bool {
	for i, e := range c.tempControllers {
		if e.Timestamp == ts {
			c.tempControllers = append(c.tempControllers[:i:i], c.tempControllers[i+1:]...)
			return true
		}
	}
	return false
}

// runCommand is GameCommand.run for one record. controller may be nil when
// the caller has no decision-maker to hand (a zone change, a phase out): a
// lose-control command then ends the control change and leaves
// controllerChangeZoneCorrection to the next state-based pass, which Java
// runs the same way (GameAction.java:1205-1207). Only the moment the
// permanent changes battlefield lists differs, and nothing resolves between.
func (g *Game) runCommand(controller PlayerController, cmd cardCommand) {
	switch cmd.Kind {
	case commandLoseControl:
		c := g.Card(cmd.Target)
		// StaticAbilityCantGainControl.cantGainControl(c) cannot be asked:
		// no CantGainControl static is evaluated yet (controlspelleffect.go
		// refuses the same).
		if c.Zone == Battlefield {
			c.removeTempController(cmd.Timestamp)
			if controller != nil {
				g.correctControllerZone(controller, cmd.Target)
			}
		}
	case commandEndEffect:
		if e := g.Card(cmd.Target); e.Zone == Command && e.IsEffect {
			g.exileEffect(cmd.Target)
		}
	}
}

// runCommands runs cmds in order, the loop shared by every run*Commands.
func (g *Game) runCommands(controller PlayerController, cmds []cardCommand) {
	for _, cmd := range cmds {
		g.runCommand(controller, cmd)
	}
}

// runLeavesPlayCommands is Card.runLeavesPlayCommands: id left the
// battlefield (GameAction.java:533, 960). The list is cleared first so a
// command that moves cards cannot run it twice.
func (g *Game) runLeavesPlayCommands(controller PlayerController, id CardID) {
	c := g.Card(id)
	cmds := c.leavesPlayCmds
	// The other lists belong to the object that left: Java's zone change
	// makes a new Card with empty ones (CR 400.7).
	c.leavesPlayCmds, c.untapCmds, c.changeControllerCmds, c.phaseOutCmds = nil, nil, nil, nil
	g.runCommands(controller, cmds)
}

// runUntapCommands is Card.runUntapCommands, run as id untaps
// (Card.java:4724). Callers check the card was tapped: Card.untap returns
// early otherwise.
func (g *Game) runUntapCommands(controller PlayerController, id CardID) {
	c := g.Card(id)
	cmds := c.untapCmds
	c.untapCmds = nil
	g.runCommands(controller, cmds)
}

// runChangeControllerCommands is Card.runChangeControllerCommands: id is
// about to change controller (GameAction.java:1023).
func (g *Game) runChangeControllerCommands(controller PlayerController, id CardID) {
	c := g.Card(id)
	cmds := c.changeControllerCmds
	c.changeControllerCmds = nil
	g.runCommands(controller, cmds)
}

// runPhaseOutCommands is Card.runPhaseOutCommands (CR 702.26f), run as id
// phases out (Card.java:5650).
func (g *Game) runPhaseOutCommands(controller PlayerController, id CardID) {
	c := g.Card(id)
	cmds := c.phaseOutCmds
	c.phaseOutCmds = nil
	g.runCommands(controller, cmds)
}

// runEndOfTurnCommands is EndOfTurn.executeUntil() at cleanup
// (PhaseHandler.java:406) followed by executeUntilEndOfPhase(playerTurn) and
// registerUntilEndCommand(playerTurn) (:407-408): the until-end-of-turn
// list, then the active player's until-the-end-of-your-next-turn commands
// registered before this turn, then the ones registered during it armed.
func (g *Game) runEndOfTurnCommands(controller PlayerController) {
	cmds := g.endOfTurnCmds
	g.endOfTurnCmds = nil
	g.runCommands(controller, cmds)

	var due []cardCommand
	kept := g.endOfNextTurnCmds[:0:0]
	for _, p := range g.endOfNextTurnCmds {
		switch {
		case p.Player != g.activePlayer:
			kept = append(kept, p)
		case p.Armed:
			p.Armed = false
			kept = append(kept, p)
		default:
			due = append(due, p.cardCommand)
		}
	}
	g.endOfNextTurnCmds = kept
	g.runCommands(controller, due)
}

// runEndOfCombatCommands is EndOfCombat.executeUntil() (PhaseHandler.
// endCombat, java:1262): the until-end-of-combat list.
func (g *Game) runEndOfCombatCommands(controller PlayerController) {
	cmds := g.endOfCombatCmds
	g.endOfCombatCmds = nil
	g.runCommands(controller, cmds)
}

// loseControlTokens are the LoseControl$ values ControlGainEffect registers
// a command for, in the order its resolve checks them.
var loseControlTokens = map[string]bool{
	"LeavesPlay": true, "Untap": true, "LoseControl": true, "EOT": true,
	"EndOfCombat": true, "UntilTheEndOfYourNextTurn": true,
}

// refusedLoseControlTokens are the LoseControl$ values whose bookkeeping
// this port does not have: StaticCommandCheck needs Card.staticCommandList
// (evaluated at every state check, GameAction.java:1180-1198) and
// UntilSourceUnattached the unattach command list.
var refusedLoseControlTokens = map[string]bool{"StaticCommandCheck": true, "UntilSourceUnattached": true}

// parseLoseControl splits LoseControl$ on "," as ControlGainEffect.resolve
// does. A value this port cannot register is an error, never silently
// dropped (GO-7).
func parseLoseControl(raw string) ([]string, error) {
	tokens := make([]string, 0, 4)
	for _, tok := range strings.Split(raw, ",") {
		if refusedLoseControlTokens[tok] || !loseControlTokens[tok] {
			return nil, fmt.Errorf("engine: GainControl: LoseControl$ %q not resolvable yet", tok)
		}
		tokens = append(tokens, tok)
	}
	return tokens, nil
}

func hasToken(tokens []string, want string) bool {
	for _, t := range tokens {
		if t == want {
			return true
		}
	}
	return false
}

// registerLoseControl is the `if (lose != null)` block of
// ControlGainEffect.resolve for one target: the lose-control command goes on
// the host's lists and the game's phase lists, as each token says.
func (g *Game) registerLoseControl(host *Card, tokens []string, target CardID, ts uint64, activator PlayerID) {
	cmd := cardCommand{Kind: commandLoseControl, Target: target, Timestamp: ts, Host: host.ID}
	if hasToken(tokens, "LeavesPlay") && host.ID != target {
		// Only return control if host and target are different cards.
		host.leavesPlayCmds = append(host.leavesPlayCmds, cmd)
	}
	if hasToken(tokens, "Untap") {
		host.untapCmds = append(host.untapCmds, cmd)
	}
	if hasToken(tokens, "LoseControl") {
		host.changeControllerCmds = append(host.changeControllerCmds, cmd)
	}
	if hasToken(tokens, "EOT") {
		g.endOfTurnCmds = append(g.endOfTurnCmds, cmd)
	}
	if hasToken(tokens, "EndOfCombat") {
		g.endOfCombatCmds = append(g.endOfCombatCmds, cmd)
	}
	if hasToken(tokens, "UntilTheEndOfYourNextTurn") {
		g.endOfNextTurnCmds = append(g.endOfNextTurnCmds, playerCommand{
			cardCommand: cmd, Player: activator,
			// registerUntilEnd during the activator's own turn, addUntilEnd
			// otherwise (ControlGainEffect.java:196-200).
			Armed: g.activePlayer == activator,
		})
	}
}
