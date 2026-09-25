package engine

//enginelint:allow game control ability effecthelpers card condition id zone valid defined

import (
	"fmt"

	"github.com/jczastkiewicz/crucible/internal/valid"
)

// removeFromMatchEffect is RemoveFromMatchEffect.java: every card in the
// game matching RemoveType$ (IncludeSideboard$ adds sideboards), or else
// the Defined$/targeted cards, ceases to exist. Java also drops them from
// the rest of the match and, with RemoveFromInventory$, from the player's
// collection -- bookkeeping outside a single game, which is all this port
// plays.
type removeFromMatchEffect struct{}

func (removeFromMatchEffect) Resolve(g *Game, a *Ability, _ PlayerController) error {
	if err := rejectParams(a, "RemoveFromMatch", "Condition"); err != nil {
		return err
	}
	source := g.Card(a.Source)
	if !subAbilityConditionMet(g, source, a.Amounts, a.Params) {
		return nil
	}
	var cards []CardID
	if spec, ok := a.Params.Param("RemoveType"); ok {
		parsed := valid.Parse(spec)
		sideboard := hasParam(a, "IncludeSideboard")
		for i := 1; i < len(g.cards); i++ {
			c := &g.cards[i]
			if c.Zone == None || (c.Zone == Sideboard && !sideboard) {
				continue
			}
			if Matches(g, c, parsed, a.Controller, a.Source) {
				cards = append(cards, CardID(i))
			}
		}
	} else {
		var err error
		if cards, err = targetedOrDefinedCards(source, a.Params, a.refs()); err != nil {
			return fmt.Errorf("engine: RemoveFromMatch: %w", err)
		}
	}
	for _, id := range cards {
		g.ceaseToExist(id)
	}
	return nil
}
