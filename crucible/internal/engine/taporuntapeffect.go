package engine

//enginelint:allow card game ability defined condition control zone

import "fmt"

// tapOrUntapUnresolvedParams are TapOrUntapEffect.java's params this port
// cannot honour yet.
// TargetingPlayer$ is targeting.go's (targetingPlayerOf).
var tapOrUntapUnresolvedParams = [...]string{
	"PresentCompare"}

// tapOrUntapEffect is TapOrUntapEffect.java: for each target
// (targetedOrDefinedCards -- 38 of 41 real lines name ValidTgts$) still on
// the battlefield, the activator decides tap or untap (ChooseTapOrUntap),
// or Toggle$ flips it. Tapping an already-tapped permanent, or untapping an
// untapped one, does nothing and fires nothing -- Card.tap/untap's own
// false return.
type tapOrUntapEffect struct{}

func (tapOrUntapEffect) Resolve(g *Game, a *Ability, controller PlayerController) error {
	if err := rejectUnreadCondition(a, a.Params.Name); err != nil {
		return err
	}
	for _, key := range tapOrUntapUnresolvedParams {
		if _, ok := a.Params.Param(key); ok {
			return fmt.Errorf("engine: TapOrUntap: %s$ not resolvable yet", key)
		}
	}
	source := g.Card(a.Source)
	if !subAbilityConditionMet(g, source, a.Amounts, a.Params) {
		return nil
	}
	cards, err := targetedOrDefinedCards(source, a.Params, a.refs())
	if err != nil {
		return fmt.Errorf("engine: TapOrUntap: %w", err)
	}
	_, toggle := a.Params.Param("Toggle")
	// Tapper$ names the player who decides (TapOrUntapEffect.java:38-42),
	// the activator when absent.
	decider := a.Controller
	if spec, ok := a.Params.Param("Tapper"); ok {
		players, err := definedPlayers(g, a.Controller, a.Source, spec, a.refs())
		if err != nil {
			return fmt.Errorf("engine: TapOrUntap: Tapper$: %w", err)
		}
		if len(players) == 0 {
			return nil
		}
		decider = players[0]
	}
	for _, id := range cards {
		c := g.Card(id)
		if c.Zone != Battlefield {
			continue
		}
		tap := !c.Tapped
		if !toggle {
			tap = controller.ChooseTapOrUntap(g, decider, id)
		}
		switch {
		case tap && !c.Tapped:
			c.Tapped = true
			g.checkTapsTriggers(controller, id, a.Controller, false)
		case !tap && c.Tapped:
			g.runUntapCommands(controller, id)
			c.Tapped = false
			g.checkUntapsTriggers(controller, id)
		}
	}
	return nil
}
