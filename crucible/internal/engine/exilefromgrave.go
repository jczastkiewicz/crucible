// Exiling a card from the graveyard or the hand as an activation cost:
// ExileFromGrave<1/CARDNAME>/ExileFromHand<1/CARDNAME>, CostExile.java's own
// graveyard-origin and hand-origin constructors (the identical class
// exile.go's own battlefield ExileCost already reads, a different ZoneType
// argument each) -- SelfExile's own two siblings for an ability activated
// from somewhere other than the battlefield (ActivationZone$ Graveyard/Hand,
// activateability.go).
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

// exileFromHand is exileFromGraveyard's own sibling for ExileFromHand<1/
// CARDNAME|NICKNAME> -- the identical no-trigger, no-LKI move, a card
// leaving the hand rather than the graveyard.
func exileFromHand(g *Game, id CardID) {
	c := g.Card(id)
	g.Move(id, Exile, c.Owner)
}
