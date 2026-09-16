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
	// Counters is player-level counters -- poison chief among them, which is
	// what CR 704.5c checks. The same type as a card's, because nothing about
	// "a count that is never stored at zero" is specific to what holds it.
	Counters Counters
	// DrewFromEmptyLibrary records an attempted draw with nothing to draw.
	// CheckStateBasedActions reads it for CR 704.5b and clears it either way
	// -- a one-shot check, the same as Java's triedToDrawFromEmptyLibrary --
	// so a player who survives this check (because a replacement effect
	// intervenes, once one exists) does not lose on the next one for a draw
	// that already happened.
	DrewFromEmptyLibrary bool
	// ManaPool is this player's own floating mana (CR 106.4, mana.go).
	// Emptied every phase/step transition (CR 500.4, emptyManaPools,
	// turn.go), not something a card ability triggers.
	ManaPool Pool
	// LandsPlayed is how many lands this player has played this turn (CR
	// 305.2), read by PlayLand's own per-turn limit. LandsPlayedLastTurn is
	// last turn's count, Java's own landsPlayedLastTurn -- no card in scope
	// reads it yet (a replacement effect keyed on "if you've played a land
	// this turn" would), but cleanupStep (turn.go) rolls it forward every
	// turn regardless, the same "reset land-bearing state whether or not a
	// reader exists yet" position CR 500.4's own mana-pool emptying is in.
	// Both reset for every player at cleanup, not just the active one (CR
	// 305.2's own scope: any player who played a land this turn, and every
	// game player's Game.onCleanupPhase in Java runs the same reset over
	// every registered player).
	LandsPlayed         int
	LandsPlayedLastTurn int
}
