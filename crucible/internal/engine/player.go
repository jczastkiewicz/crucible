// Players.

package engine

// Player is one player in one game.
//
// Like Card it holds no back-reference. Its library, hand and graveyard live
// in the game's zone table rather than on the player, because a zone is
// addressed by (type, owner) from a dozen places that have the pair and not
// the player.
type Player struct {
	// ID is this player's own handle.
	ID PlayerID
	// Name is for reports and event logs, never for identity (GO-9).
	Name string
	// Life is the current total. It goes negative before the state-based
	// action that ends the game runs, so it is signed.
	Life int
	// Turn is how many turns this player has taken, which several card scripts
	// count.
	Turn int
	// Lost records that the player has left the game, and Won that they won
	// it. Both can be false at once; both true is an engine invariant breach.
	Lost bool
	Won  bool
}
