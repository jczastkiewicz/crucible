package engine

//enginelint:allow id card game ability defined condition control parts animate zone zonemove effecthelpers cardtype

import (
	"fmt"

	"github.com/jczastkiewicz/crucible/internal/cardtype"
)

// assimilateEffect is AssimilateEffect.java: each Defined$/targeted card ends
// up under the activator's control as a Borg artifact creature with a +1/+1
// counter. A permanent only changes hands, gets the counter and the types in
// place; a card in a hidden or non-battlefield zone enters the battlefield
// with the counter and takes the types as it enters, so an "artifact enters"
// trigger sees it as one (AssimilateEffect.java:57-74).
//
// Ported from forge-game/src/main/java/forge/game/ability/effects/AssimilateEffect.java's resolve.
type assimilateEffect struct{}

func (assimilateEffect) Resolve(g *Game, a *Ability, c PlayerController) error {
	source := g.Card(a.Source)
	if !subAbilityConditionMet(g, source, a.Amounts, a.Params) {
		return nil
	}
	cards, err := targetedOrDefinedCards(source, a.Params, a.refs())
	if err != nil {
		return fmt.Errorf("engine: Assimilate: %w", err)
	}
	// AssimilateEffect.ANIMATE: Mode$ Continuous | AddType$ Artifact &
	// Creature & Borg | RemoveCreatureTypes$ True.
	drop, ok := subtypeCategoryDrop(g, false, true, false, false)
	if !ok {
		return fmt.Errorf("engine: Assimilate: RemoveCreatureTypes$ needs the subtype vocabulary, which this game's DB lacks")
	}
	tmpl := animateRecord{Permanent: true}
	tmpl.Types.AddTypes = cardtype.ParseToken("Artifact").Union(cardtype.ParseToken("Creature")).Union(cardtype.ParseToken("Borg"))
	tmpl.Types.DropSubtype = drop

	for _, id := range cards {
		card := g.Card(id)
		if card == nil || card.IsPhasedOut() {
			continue
		}
		// Before any move, or the card would enter under the wrong
		// controller (AssimilateEffect.java:52-56).
		if card.Zone == Battlefield && card.Controller() != a.Controller {
			g.changeController(c, id, a.Controller)
		}
		if card.Zone == Battlefield {
			if n := g.countersReplaced(c, a.Controller, CardEntity(id), P1P1, 1); n > 0 {
				g.addCardCounters(c, a.Controller, a.Source, id, P1P1, n)
			}
			g.timestamp++
			r := tmpl
			r.Card, r.Timestamp = id, g.timestamp
			g.addAnimate(r)
			continue
		}
		g.moveByEffect(c, id, Battlefield, 0, a.Controller, false, enterCounters{kind: P1P1, n: 1, placer: a.Controller})
		if card.Zone == Battlefield {
			g.applyChangeZoneStatic(&tmpl, id)
		}
	}
	return nil
}
