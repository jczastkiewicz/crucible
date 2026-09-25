package engine

//enginelint:allow game control ability effecthelpers card condition defined id zone

import "fmt"

// removeFromGameEffect is RemoveFromGameEffect.java: each Defined$ (or
// targeted) card ceases to exist -- GameAction.ceaseToExist with skipTrig,
// so no leaves-the-battlefield trigger runs. The card moves to ZoneType
// None, the zone Java parks it in.
type removeFromGameEffect struct{}

func (removeFromGameEffect) Resolve(g *Game, a *Ability, _ PlayerController) error {
	if err := rejectParams(a, "RemoveFromGame", "Condition"); err != nil {
		return err
	}
	source := g.Card(a.Source)
	if !subAbilityConditionMet(g, source, a.Amounts, a.Params) {
		return nil
	}
	cards, err := targetedOrDefinedCards(source, a.Params, a.refs())
	if err != nil {
		return fmt.Errorf("engine: RemoveFromGame: %w", err)
	}
	for _, id := range cards {
		g.ceaseToExist(id)
	}
	return nil
}

// ceaseToExist is GameAction.ceaseToExist(c, true): id leaves whatever zone
// it is in -- the stack included, dropping the spell -- for ZoneType None,
// with no zone-change trigger.
func (g *Game) ceaseToExist(id CardID) {
	c := g.Card(id)
	if c.Zone == None {
		return
	}
	if c.Zone == Stack {
		kept := g.stack[:0]
		for _, s := range g.stack {
			if s.Source != id {
				kept = append(kept, s)
			}
		}
		g.stack = kept
	}
	if c.Zone == Battlefield {
		g.removeFromCombat(id)
	}
	g.Move(id, None, c.Owner)
}
