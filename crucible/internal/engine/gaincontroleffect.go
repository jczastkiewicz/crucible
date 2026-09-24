package engine

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
	"Condition", "ConditionDefined", "SorcerySpeed", "PlayerTurn", "Ultimate",
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
		g.changeController(id, newController)
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
// real change the permanent leaves combat and is summoning sick again.
func (g *Game) changeController(id CardID, to PlayerID) {
	c := g.Card(id)
	before := c.Controller()
	g.timestamp++
	c.tempControllers = append(c.tempControllers, ControlEffect{Timestamp: g.timestamp, Controller: to})
	if c.Controller() != before {
		c.SummonSick = true
		g.removeFromCombat(id)
	}
}
