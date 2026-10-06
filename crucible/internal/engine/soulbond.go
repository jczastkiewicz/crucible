// Soulbond pairing (CR 702.95): Card.pairedWith, the Bond effect that sets
// it, and the four places Forge clears it.
//
// Ported from forge-game/src/main/java/forge/game/ability/effects/
// BondEffect.java, Card.java:1390-1398 (getPairedWith/setPairedWith/isPaired),
// GameAction.java:622-628 (leaving the battlefield), :1008-1022 (a controller
// change), :1209-1215 (the state-based check) and Card.java:5654 (phasing out),
// and CardProperty.java:564-570 (the Paired/PairedWith properties). The
// keyword's two triggers are keyword/expand.go's Soulbond case.

package engine

//enginelint:allow id card game ability defined control zone effecthelpers

import (
	"fmt"

	"github.com/jczastkiewicz/crucible/internal/cardtype"
)

// IsPaired is Card.isPaired: the creature is Soulbond-paired.
func (c *Card) IsPaired() bool { return c.pairedWith != NoCard }

// PairedWith is Card.getPairedWith: the creature this one is paired with,
// NoCard for none.
func (c *Card) PairedWith() CardID { return c.pairedWith }

// unpair breaks id's pair: both creatures forget each other
// (GameAction.java:1015-1020).
func (g *Game) unpair(id CardID) {
	c := g.Card(id)
	if c.pairedWith == NoCard {
		return
	}
	g.Card(c.pairedWith).pairedWith = NoCard
	c.pairedWith = NoCard
}

// unpairOnLeave is GameAction.java:622-628, run as id leaves the battlefield:
// the partner is always freed, but the leaving card keeps its own link unless
// it is a real card -- a token's stale pairedWith (PORT-7: Java reproduces
// it, and a token ceases to exist, so only last-known information shows it).
func (g *Game) unpairOnLeave(id CardID) {
	c := g.Card(id)
	if c.pairedWith == NoCard {
		return
	}
	g.Card(c.pairedWith).pairedWith = NoCard
	if !c.IsToken {
		c.pairedWith = NoCard
	}
}

// unpairInvalid is GameAction.java:1209-1215, in the state-based check: a
// paired creature whose partner is no longer a creature, is controlled by
// another player or has left the battlefield is unpaired (CR 702.95e). It
// walks each player's battlefield, phased-in cards only, as Java does.
func (g *Game) unpairInvalid() {
	for _, pid := range g.Players() {
		for _, id := range g.Zone(Battlefield, pid).Cards() {
			c := g.Card(id)
			if !c.IsPaired() || !c.Type().Has(cardtype.Creature) {
				continue
			}
			partner := g.Card(c.pairedWith)
			if !partner.Type().Has(cardtype.Creature) || c.Controller() != partner.Controller() || c.Zone != Battlefield {
				g.unpair(id)
			}
		}
	}
}

// bondEffect is BondEffect.java: the activator pairs each targeted or
// Defined$ creature they control that is unpaired with a creature of their
// choosing from ValidCards$, optionally (Soulbond's two triggers, one per
// creature that can enter).
//
// Not ported: BondEffect.java:28's equalsWithGameTimestamp guard, which skips
// a creature that left and came back since the trigger fired. An ability does
// not record the zoneStamp of a Defined$ triggered card (only of its targets),
// so a creature that re-entered in between still pairs.
type bondEffect struct{}

func (bondEffect) Resolve(g *Game, a *Ability, controller PlayerController) error {
	source := g.Card(a.Source)
	p := a.Controller
	cards, err := targetedOrDefinedCards(source, a.Params, a.refs())
	if err != nil {
		return fmt.Errorf("engine: Bond: %w", err)
	}
	spec, ok := a.Params.Param("ValidCards")
	if !ok {
		return fmt.Errorf("engine: Bond: ValidCards$ is missing")
	}
	for _, id := range cards {
		c := g.Card(id)
		if c.Zone != Battlefield || c.IsPaired() || !c.Type().Has(cardtype.Creature) || c.Controller() != p {
			continue
		}
		// p.getCreaturesInPlay(): the creatures p controls on the
		// battlefield.
		var creatures []CardID
		for _, cid := range g.Zone(Battlefield, p).Cards() {
			if g.Card(cid).Type().Has(cardtype.Creature) {
				creatures = append(creatures, cid)
			}
		}
		options := filterValid(g, creatures, spec, p, a.Source)
		if len(options) == 0 {
			continue
		}
		chosen := controller.ChooseCardsForEffect(g, p, a.Source, options, 0, 1)
		if err := checkChoice(chosen, options, 0, 1); err != nil {
			return fmt.Errorf("engine: Bond: %w", err)
		}
		if len(chosen) == 0 {
			continue
		}
		g.Card(id).pairedWith = chosen[0]
		g.Card(chosen[0]).pairedWith = id
	}
	return nil
}
