package engine

//enginelint:allow ability control game effecthelpers card condition defined

import "fmt"

// gainOwnershipEffect is OwnershipGainEffect.java: the first DefinedPlayer$
// (targeted or defined; the activator when it names nobody) becomes the
// owner of each Defined$ or targeted card -- Player.changeOwnership, which
// sets the owner and leaves the card where it is (ante bookkeeping aside,
// which this port has no ante for). TgtZone$ is read at targeting only
// (TargetRestrictions.java:152-154), never here: Darkpact targets a card in
// the Ante zone.
type gainOwnershipEffect struct{}

func (gainOwnershipEffect) Resolve(g *Game, a *Ability, _ PlayerController) error {
	if err := rejectUnreadCondition(a, a.Params.Name); err != nil {
		return err
	}
	source := g.Card(a.Source)
	if !subAbilityConditionMet(g, source, a.Amounts, a.Params) {
		return nil
	}
	cards, err := targetedOrDefinedCards(source, a.Params, a.refs())
	if err != nil {
		return fmt.Errorf("engine: GainOwnership: %w", err)
	}
	owner := a.Controller
	if def, ok := a.Params.Param("DefinedPlayer"); ok {
		players, err := definedPlayers(g, a.Controller, a.Source, def, a.refs())
		if err != nil {
			return fmt.Errorf("engine: GainOwnership: %w", err)
		}
		if len(players) > 0 {
			owner = players[0]
		}
	}
	for _, id := range cards {
		g.Card(id).Owner = owner
	}
	return nil
}
