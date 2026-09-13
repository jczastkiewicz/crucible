// State-based actions and the game-over detection that rides along with
// them.

package engine

// CheckStateBasedActions applies every state-based action this port checks
// today and reports whether the game has ended as a result.
//
// Ported from forge-game/src/main/java/forge/game/GameAction.java
// (checkGameOverCondition) and forge/game/player/Player.java
// (checkLoseCondition). Only two of Java's checks are here: CR 704.5a (a
// player at zero or less life loses) and CR 704.5c (ten or more poison
// counters loses). Every other rule in Java's loop -- lethal damage, zero
// toughness, an aura with nothing to enchant, a planeswalker at zero loyalty
// -- needs either the continuous-effect layer system to compute a
// characteristic (P/T, loyalty) or a permanent type (Aura, Planeswalker,
// Battle) this port has not built. A rule this port has not reached simply
// never fires, the same as it would in a real game with no permanent that
// rule applies to.
//
// Java's own checkStateEffects loops up to nine times, because one SBA firing
// can make another one true (destroying a creature can, in turn, empty an
// Aura's target). Neither rule here can trigger the other or be triggered by
// anything else this port has, so one pass is complete; the loop returns when
// a second rule that can cascade lands.
func CheckStateBasedActions(g *Game) bool {
	if g.over {
		return true
	}

	for _, id := range g.Players() {
		p := g.Player(id)
		if p.Lost {
			continue
		}
		if p.Life <= 0 || p.Counters.Count(Poison) >= 10 {
			p.Lost = true
		}
	}

	remaining := NoPlayer
	count := 0
	for _, id := range g.Players() {
		if !g.Player(id).Lost {
			count++
			remaining = id
		}
	}

	// CR 104.2a: one player left standing wins. Zero is a draw, and more than
	// one means the game goes on -- both leave g.over false.
	switch count {
	case 1:
		g.Player(remaining).Won = true
		g.over = true
	case 0:
		g.over = true
	}
	return g.over
}
