// Encode (Cipher, CR 702.99): the resolving spell's card may be exiled and
// encoded on a creature its controller controls. The keyword's static
// (keyword.Expand) gives that creature, while the card stays encoded on it,
// "whenever this deals combat damage to a player, you may cast a copy of the
// encoded card without paying its mana cost"; the compiler appends the
// DB$ Encode to the card's spell (compile/cipher.go).
//
// Ported from forge-game/src/main/java/forge/game/ability/effects/
// EncodeEffect.java:33-74. Java keeps Card.encodedCards on the creature and
// Card.encodingCard on the exiled card, cleared as either changes zones
// (GameAction.java:454-463); here the exiled card holds one mark with both
// cards' zone stamps, so the link ends the moment either object does
// (CR 400.7), with no cleanup to run.

package engine

//enginelint:allow id card game zone control ability

import "github.com/jczastkiewicz/crucible/internal/cardtype"

type encodeEffect struct{}

// Resolve is EncodeEffect.resolve: nothing for a token or with no creature
// its controller controls; otherwise the activator may exile the card, then
// chooses the creature it is encoded on. Java exiles the card before the
// choice, so a creature that left during the exile leaves it exiled and
// unencoded.
func (encodeEffect) Resolve(g *Game, a *Ability, controller PlayerController) error {
	host := g.Card(a.Source)
	if host.IsToken {
		return nil
	}
	var choices []CardID
	for _, id := range g.Zone(Battlefield, host.Controller()).Cards() {
		if g.Card(id).Type().Has(cardtype.Creature) {
			choices = append(choices, id)
		}
	}
	if len(choices) == 0 || !controller.ConfirmEffect(g, a.Controller, a.Source) {
		return nil
	}
	g.Move(a.Source, Exile, host.Owner)
	var live []CardID
	for _, id := range choices {
		if c := g.Card(id); c.Zone == Battlefield && c.Type().Has(cardtype.Creature) {
			live = append(live, id)
		}
	}
	if host.Zone != Exile || len(live) == 0 {
		return nil
	}
	chosen := controller.ChooseCardsForEffect(g, a.Controller, a.Source, live, 1, 1)
	if len(chosen) != 1 {
		return nil
	}
	host.encoded = encodeMark{creature: chosen[0], creatureStamp: g.Card(chosen[0]).zoneStamp, stamp: host.zoneStamp}
	return nil
}

// encodedOn is the creature cipher is encoded on, false once either has left
// the zone it was in when the encoding was made.
func (g *Game) encodedOn(cipher CardID) (CardID, bool) {
	c := g.Card(cipher)
	m := c.encoded
	if m.creature == NoCard || c.Zone != Exile || c.zoneStamp != m.stamp {
		return NoCard, false
	}
	cr := g.Card(m.creature)
	if cr.Zone != Battlefield || cr.zoneStamp != m.creatureStamp {
		return NoCard, false
	}
	return m.creature, true
}
