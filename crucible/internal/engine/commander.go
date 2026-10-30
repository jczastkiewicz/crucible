// The Commander designation (ADR-0051): Player.addCommander / removeCommander
// (Player.java:2838-2854) and Card.setCommander, as AlterAttribute's Commander
// attribute calls them. Java's addCommander also builds the Commander Effect
// card (903.9b replacement, cast permission); that, tax, cast counts and
// commander damage are the format and are not ported.

package engine

//enginelint:allow id card player game

import "slices"

// addCommander makes id one of owner's commanders, once.
func (g *Game) addCommander(owner PlayerID, id CardID) {
	p := g.Player(owner)
	if !slices.Contains(p.commanders, id) {
		p.commanders = append(p.commanders, id)
	}
	g.Card(id).commander = true
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
