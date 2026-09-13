// State-based actions and the game-over detection that rides along with
// them.

package engine

// CheckStateBasedActions applies every state-based action this port checks
// today and reports whether the game has ended as a result.
//
// Ported from forge-game/src/main/java/forge/game/GameAction.java
// (checkGameOverCondition, stateBasedAction704_5q) and
// forge/game/player/Player.java (checkLoseCondition). Three of Java's checks
// are here: CR 704.5a (a player at zero or less life loses), CR 704.5c (ten
// or more poison counters loses), and CR 704.5q (a permanent carrying both
// +1/+1 and -1/-1 counters loses the smaller pile from each, in equal
// number). Every other rule in Java's loop -- lethal damage, zero toughness,
// an aura with nothing to enchant, a planeswalker at zero loyalty -- needs
// either the continuous-effect layer system to compute a characteristic
// (P/T, loyalty) or a permanent type (Aura, Planeswalker, Battle) this port
// has not built. A rule this port has not reached simply never fires, the
// same as it would in a real game with no permanent that rule applies to.
//
// Java's `canRemoveCounters` guard on 704.5q -- some cards grant "counters
// can't be removed from CARDNAME" -- is a static ability, so it is not
// checked either: nothing this port can build yet grants that effect, so its
// absence changes no card's behaviour today.
//
// Java's own checkStateEffects loops up to nine times, because one SBA firing
// can make another one true (destroying a creature can, in turn, empty an
// Aura's target). None of the three rules here can trigger each other or be
// triggered by anything else this port has, so one pass is complete; the loop
// returns when a rule that can cascade lands.
//
// A game that has already ended skips 704.5q entirely, the same as Java:
// checkStateEffects returns as soon as checkGameOverCondition finds the game
// over, before its creature loop ever runs.
func CheckStateBasedActions(g *Game) bool {
	if g.over {
		return true
	}

	// CR 704.5a, 704.5c
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
	if g.over {
		return true
	}

	// CR 704.5q
	for _, pid := range g.Players() {
		for _, id := range g.Zone(Battlefield, pid).Cards() {
			annihilateCounters(g.Card(id))
		}
	}
	return false
}

// annihilateCounters is CR 704.5q: N +1/+1 and N -1/-1 counters are removed
// together, where N is the smaller pile. A card carrying only one kind, or
// neither, is untouched.
func annihilateCounters(c *Card) {
	plus, minus := c.Counters.Count(P1P1), c.Counters.Count(M1M1)
	if plus == 0 || minus == 0 {
		return
	}
	remove := plus
	if minus < remove {
		remove = minus
	}
	c.Counters.Add(P1P1, -remove)
	c.Counters.Add(M1M1, -remove)
}
