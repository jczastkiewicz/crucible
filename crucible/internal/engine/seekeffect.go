package engine

//enginelint:allow ability card condition control defined effecthelpers game id parts player valid zone triggerpack

import (
	"fmt"
	"strings"

	"github.com/jczastkiewicz/crucible/internal/valid"
)

// seekEffect is SeekEffect.java: for each target or Defined$ player
// (default You), per Types$ entry (or Type$, default Card), Num$ (default
// 1) cards picked at random from their library among those matching the
// type go to their hand -- Aggregates.random(pool, n)'s reservoir sample on
// the game's own stream. RememberFound$/ImprintFound$ record the cards that
// reached a hand; ChangesZoneAll fires once for the whole seek. DefinedCards$
// replaces the library as the pool (Top_10_OfLibrary: the activator's top ten).
type seekEffect struct{}

func (seekEffect) Resolve(g *Game, a *Ability, controller PlayerController) error {
	if err := rejectUnreadCondition(a, a.Params.Name); err != nil {
		return err
	}
	source := g.Card(a.Source)
	if !subAbilityConditionMet(g, source, a.Amounts, a.Params) {
		return nil
	}
	types := []string{"Card"}
	if raw, ok := a.Params.Param("Types"); ok {
		types = strings.Split(raw, ",")
	} else if raw, ok := a.Params.Param("Type"); ok {
		types = []string{raw}
	}
	num, err := optionalAmount(g, a, "Seek", "Num", 1)
	if err != nil || num <= 0 {
		return err
	}
	seekers, err := targetedOrDefinedPlayers(g, a.Controller, a.Source, a.Params, a.refs())
	if err != nil {
		return err
	}
	var moved []CardID
	for _, seeker := range g.inAPNAPOrder(seekers) {
		if g.Player(seeker).Lost {
			continue
		}
		var sought []CardID
		for _, t := range types {
			pool := append([]CardID(nil), g.Zone(Library, seeker).Cards()...)
			if def, ok := a.Params.Param("DefinedCards"); ok {
				var err error
				if pool, err = definedCards(source, def, a.refs()); err != nil {
					return fmt.Errorf("engine: Seek: DefinedCards$: %w", err)
				}
			}
			if t != "Card" {
				spec := valid.Parse(t)
				kept := pool[:0]
				for _, id := range pool {
					if Matches(g, g.Card(id), spec, source.Controller(), a.Source) {
						kept = append(kept, id)
					}
				}
				pool = kept
			}
			for _, i := range g.randomSample(len(pool), num) {
				id := pool[i]
				g.moveByEffect(controller, id, Hand, 0, NoPlayer, false)
				moved = append(moved, id)
				if g.Card(id).Zone == Hand {
					sought = append(sought, id)
				}
			}
		}
		for _, id := range sought {
			if hasParam(a, "RememberFound") {
				source.Memory.Remember(CardEntity(id))
			}
			if hasParam(a, "ImprintFound") {
				source.Memory.Imprint(id)
			}
		}
		if len(sought) > 0 {
			g.checkSeekAllTriggers(controller, seeker, sought) // SeekEffect.java:105-107
		}
	}
	g.checkChangesZoneAllTriggers(controller, moved, Library, Hand)
	return nil
}
