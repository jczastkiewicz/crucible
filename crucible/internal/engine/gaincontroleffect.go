package engine

//enginelint:allow id card game ability defined condition control parts zone

import "fmt"

// gainControlUnresolvedParams are ControlGainEffect.java's params this port
// cannot honour yet: every duration-scoped change (LoseControl$ and its
// StaticCommand*$ checks), until-end-of-turn keywords (AddKWs$), the choice
// and sweep shapes (Choices$, Chooser$, AllValid$), and target-selection
// restrictions targeting.go does not enforce.
var gainControlUnresolvedParams = [...]string{
	"LoseControl", "StaticCommandCheckSVar", "StaticCommandSVarCompare", "AddKWs",
	"Choices", "Chooser", "AllValid",
	"TargetsForEachPlayer", "TargetsWithControllerProperty", "TargetsAtRandom",
	"TargetingPlayerControls", "TargetingPlayer", "MaxTotalTargetCMC",
	"Condition", "SorcerySpeed", "Ultimate",
}

// gainControlEffect is ControlGainEffect.java's permanent shape: the
// NewController$ player (default the activator) gains control of each
// targeted or Defined$ permanent (default Self) with no end date --
// Card.addTempController, a timestamped entry Controller merges with
// Layer 2's continuous effects (Card.tempControllers). Untap$ untaps it;
// Optional$ asks per permanent. A permanent that changes controller is
// removed from combat and is summoning sick again (CR 506.4, 302.6).
type gainControlEffect struct{}

func (gainControlEffect) Resolve(g *Game, a *Ability, controller PlayerController) error {
	for _, key := range gainControlUnresolvedParams {
		if _, ok := a.Params.Param(key); ok {
			return fmt.Errorf("engine: GainControl: %s$ not resolvable yet", key)
		}
	}
	source := g.Card(a.Source)
	if !subAbilityConditionMet(g, source, a.Amounts, a.Params) {
		return nil
	}
	newController := a.Controller
	if spec, ok := a.Params.Param("NewController"); ok {
		players, err := definedPlayers(g, a.Controller, a.Source, spec, a.refs())
		if err != nil {
			return fmt.Errorf("engine: GainControl: NewController$: %w", err)
		}
		if len(players) > 0 {
			newController = players[0]
		}
	}
	cards, err := targetedOrDefinedCards(source, a.Params, a.refs())
	if err != nil {
		return fmt.Errorf("engine: GainControl: %w", err)
	}
	_, untap := a.Params.Param("Untap")
	_, optional := a.Params.Param("Optional")
	_, remember := a.Params.Param("RememberControlled")
	_, forget := a.Params.Param("ForgetControlled")
	for _, id := range cards {
		if g.Card(id).Zone != Battlefield {
			continue
		}
		if optional && !controller.ConfirmEffect(g, a.Controller, a.Source) {
			continue
		}
		g.changeController(controller, id, newController)
		c := g.Card(id)
		if untap && c.Tapped {
			c.Tapped = false
			g.checkUntapsTriggers(controller, id)
		}
		if remember {
			source.Memory.Remember(CardEntity(id))
		}
		if forget {
			source.Memory.Forget(CardEntity(id))
		}
	}
	return nil
}

// changeController gives id a new timestamped controller (Java's
// addTempController plus controllerChangeZoneCorrection): when that is a
// real change the permanent moves to its controller's battlefield list,
// leaves combat and is summoning sick again (correctControllerZone).
func (g *Game) changeController(controller PlayerController, id CardID, to PlayerID) {
	g.timestamp++
	g.changeControllerAt(controller, id, to, g.timestamp)
}

// changeControllerAt is changeController under a timestamp the caller took
// -- several permanents changing controller in one event share it.
func (g *Game) changeControllerAt(controller PlayerController, id CardID, to PlayerID, ts uint64) {
	c := g.Card(id)
	before := c.Controller()
	c.tempControllers = append(c.tempControllers, ControlEffect{Timestamp: ts, Controller: to})
	if c.Zone == Battlefield {
		g.correctControllerZone(controller, id)
	} else if c.Controller() != before {
		// A spell on the stack (ControlSpell) has no battlefield list to
		// move between; it only takes the flags.
		c.SummonSick = true
		g.removeFromCombat(id)
		g.loseRingBearer(id)
	}
}

// correctControllerZone is GameAction.controllerChangeZoneCorrection
// (GameAction.java:994-1033, ADR-0037): a battlefield permanent whose
// Controller() is no longer the owner of the list it sits in moves to the end
// of its controller's battlefield list, so Zone(Battlefield, pid) is the
// permanents pid controls. The card keeps its Timestamp and zoneStamp (CR
// 400.7: the same object) and fires neither a ZoneChanged event nor a
// ChangesZone trigger -- Java suppresses the latter -- and a phased-out card
// stays phased out. The permanent leaves combat, is summoning sick under its
// new controller and stops being its old controller's Ring-bearer, for a
// Layer 2 change as for a one-shot one, then Mode$ ChangesController triggers
// fire (checkChangesControllerTriggers). Java's runChangeControllerCommands
// and the Soulbond unpairing (game-state.md, "Not ported yet") are not done.
func (g *Game) correctControllerZone(controller PlayerController, id CardID) {
	c := g.Card(id)
	if c.Zone != Battlefield {
		return
	}
	to := c.Controller()
	if to == c.ZoneOwner {
		return
	}
	original := c.ZoneOwner
	g.Zone(Battlefield, c.ZoneOwner).remove(id)
	c.ZoneOwner = to
	g.Zone(Battlefield, to).cards.Add(id)
	if c.phasedOut != NoPlayer {
		g.setPhasedOut(id, c.phasedOut)
	}
	c.SummonSick = true
	c.cameUnderControl = true
	g.removeFromCombat(id)
	g.loseRingBearer(id)
	g.checkChangesControllerTriggers(controller, id, original)
}

// correctControllerZones runs correctControllerZone over every battlefield
// phased-in permanent whose controller differs from its list's owner, in
// Java's walk order (each player's list in turn, p.getCardsIn(Battlefield)
// skipping phased-out cards: a permanent that phased out with its Aura is not
// re-homed while the Aura's static is off). It collects first and moves after, so
// no list is changed while it is being read. CheckStateBasedActions runs it
// right after applyContinuousControl.
func (g *Game) correctControllerZones(controller PlayerController) {
	var moved []CardID
	for _, pid := range g.Players() {
		for _, id := range g.Zone(Battlefield, pid).Cards() {
			if g.Card(id).Controller() != pid {
				moved = append(moved, id)
			}
		}
	}
	for _, id := range moved {
		g.correctControllerZone(controller, id)
	}
}
