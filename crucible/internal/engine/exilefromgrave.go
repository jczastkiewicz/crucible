// Exiling a card from the graveyard as an activation cost:
// ExileFromGrave<1/CARDNAME>, CostExile.java's own graveyard-origin
// constructor (the identical class exile.go's own battlefield ExileCost
// already reads, a different ZoneType argument) -- SelfExile's sibling for
// an ability activated from the graveyard rather than the battlefield
// (ActivationZone$ Graveyard, activateability.go).
package engine

// exileFromGraveyard moves id from the graveyard to exile as a paid cost.
// Unlike exileCards (exile.go), this fires no trigger check at all: CR
// 603.6d's own "leaves the battlefield" family (checkExiledTriggers) is
// specifically about a permanent leaving the battlefield, which a graveyard
// card never was for this move, and no general "leaves the graveyard"
// trigger family exists in this port at all yet (56 real corpus T:Mode$
// ChangesZone | Origin$ Graveyard lines, an unrelated, unbuilt gap --
// game-state.md's own "Not ported yet"). g.LKI is not frozen either, the
// identical reason: that snapshot exists for a battlefield departure's own
// dying-state lookback, which nothing here needs.
func exileFromGraveyard(g *Game, id CardID) {
	c := g.Card(id)
	g.Move(id, Exile, c.Owner)
}
