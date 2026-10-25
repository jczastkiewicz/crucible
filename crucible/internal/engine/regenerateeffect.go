package engine

//enginelint:allow id card game ability additional defined condition control zone

import (
	"fmt"
	"strings"
)

// regenerateEffect is RegenerateEffect.java: each targeted or Defined$
// permanent (default Self) still on the battlefield gets one regeneration
// shield (Card.RegenShields) until end of turn; Game.regenerate spends it on
// the next destruction. Java builds one Regeneration effect card per
// resolution, each replacing one destruction per remembered permanent, so a
// counter is the same observable state. 262 real lines.
type regenerateEffect struct{}

func (regenerateEffect) Resolve(g *Game, a *Ability, _ PlayerController) error {
	if err := rejectUnreadCondition(a, a.Params.Name); err != nil {
		return err
	}
	source := g.Card(a.Source)
	if !subAbilityConditionMet(g, source, a.Amounts, a.Params) {
		return nil
	}
	// getDefinedCardsOrTargeted: a Defined$ wins over the targets
	// (SpellAbilityEffect.java:271-304), Soldevi Sentry's targeted opponent.
	var cards []CardID
	var err error
	if defined, ok := a.Params.Param("Defined"); ok {
		cards, err = definedCards(source, defined, a.refs())
	} else {
		cards, err = targetedOrDefinedCards(source, a.Params, a.refs())
	}
	if err != nil {
		return fmt.Errorf("engine: Regenerate: %w", err)
	}
	var follow *regenFollow
	if subs := additionalAbilities(a.Params, "RegenerationAbility"); len(subs) > 0 {
		follow = &regenFollow{follow: subs[0], controller: a.Controller, source: a.Source, amounts: a.Amounts}
		if raw, ok := a.Params.Param("RememberObjects"); ok {
			for _, def := range strings.Split(raw, " & ") {
				es, err := definedEntities(g, a.Controller, source, def, a.refs())
				if err != nil {
					return fmt.Errorf("engine: Regenerate: %w", err)
				}
				follow.remember = append(follow.remember, es...)
			}
		}
	} else if _, ok := a.Params.Param("RememberObjects"); ok {
		return fmt.Errorf("engine: Regenerate: RememberObjects$ without RegenerationAbility$ not resolvable yet")
	}
	for _, id := range cards {
		if c := g.Card(id); c.Zone == Battlefield {
			c.RegenShields++
			if follow != nil {
				c.regenFollows = append(append([]regenFollow(nil), c.regenFollows...), *follow)
			}
		}
	}
	return nil
}
