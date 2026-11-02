// The Commander designation (ADR-0051): Player.addCommander / removeCommander
// (Player.java:2838-2854) and Card.setCommander, as AlterAttribute's Commander
// attribute calls them. addCommander also makes the owner's Commander Effect
// card once (Player.createCommanderEffect, ADR-0053): its 903.9b Moved
// replacement sends a commander bound for the hand or library to the Command
// zone, its MayPlay$ static lets the owner cast it from there, and the cast
// is counted (castrecord.go) and taxed (commanderTax). Commander damage, the
// variants and their 903.9a state-based action are not ported.

package engine

//enginelint:allow id card player game zone

import "slices"

// addCommander makes id one of owner's commanders, once.
func (g *Game) addCommander(owner PlayerID, id CardID) {
	p := g.Player(owner)
	if !slices.Contains(p.commanders, id) {
		p.commanders = append(p.commanders, id)
	}
	if p.commanderEffect == NoCard {
		// Permanent like the monarch's card: a zero lifetime would end it at
		// the first cleanup.
		p.commanderEffect = g.putDesignationCard(NoCard, owner, g.db.CommanderEffect)
	}
	g.Card(id).commander = true
}

// IsCommander is Card.isCommander for id (ADR-0051).
func (g *Game) IsCommander(id CardID) bool { return g.Card(id).isCommander() }

// AddCommander makes id, a card owner owns, one of owner's commanders: the
// Commander setup a test or fixture gives a game (ADR-0053).
func (g *Game) AddCommander(owner PlayerID, id CardID) { g.addCommander(owner, id) }

// commanderTax is the CostAdjustment.java:56-61 step: two generic mana for
// each time pid cast card, a commander cast from the Command zone, before.
func (g *Game) commanderTax(pid PlayerID, c *Card) int {
	if !c.isCommander() || c.Zone != Command {
		return 0
	}
	return 2 * g.commanderCastsOf(pid, c.ID)
}

// removeCommander is Player.removeCommander: id is no longer a commander.
func (g *Game) removeCommander(owner PlayerID, id CardID) {
	p := g.Player(owner)
	p.commanders = slices.DeleteFunc(p.commanders, func(c CardID) bool { return c == id })
	g.Card(id).commander = false
}

// Commanders is pid's commanders, in the order they were designated.
func (g *Game) Commanders(pid PlayerID) []CardID {
	return slices.Clone(g.Player(pid).commanders)
}
