// State-based actions and the game-over detection that rides along with
// them.

package engine

// CheckStateBasedActions applies every state-based action this port checks
// today and reports whether the game has ended as a result.
//
// Ported from forge-game/src/main/java/forge/game/GameAction.java
// (checkGameOverCondition, stateBasedAction704_5q) and
// forge/game/player/Player.java (checkLoseCondition). Six of Java's checks
// are here: CR 704.5b (an attempted draw with nothing to draw loses), CR
// 704.5a (a player at zero or less life loses), CR 704.5c (ten or more
// poison counters loses), CR 704.5q (a permanent carrying both +1/+1 and
// -1/-1 counters loses the smaller pile from each, in equal number), and a
// partial CR 704.5f/704.5m (an Aura not attached to anything on the
// battlefield goes to its owner's graveyard; an Equipment or Fortification
// attached to something no longer on the battlefield becomes unattached).
// Every other rule in Java's loop -- lethal damage, zero toughness, a
// planeswalker at zero loyalty, and the rest of 704.5f/704.5m's own
// legality (an Aura's own "Enchant" restriction being violated by something
// other than its host leaving, protection, hexproof) -- needs either the
// continuous-effect layer system to compute a characteristic (P/T, loyalty)
// or a permanent type (Planeswalker, Battle) this port has not built, or a
// valid-string evaluator to check a restriction this port does not have
// (game-state.md's "Not ported yet", `internal/valid`'s own doc comment). A
// rule this port has not reached simply never fires, the same as it would
// in a real game with no permanent that rule applies to.
//
// 704.5b is checked first, matching Java's own order -- its comment cites
// Lich's Mirror (CR 704.7), a card not ported, so today's checks would give
// the same result in any order. Kept anyway: it costs nothing, and a future
// port of that card should not have to notice these were ever reordered.
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
//
// A GameEnded event fires exactly once, on the call that flips g.over --
// never on a later call finding it already true, and not from any other
// path: this is the only place g.over is set.
func CheckStateBasedActions(g *Game) bool {
	if g.over {
		return true
	}

	// CR 704.5b, 704.5a, 704.5c
	for _, id := range g.Players() {
		p := g.Player(id)
		drewFromEmpty := p.DrewFromEmptyLibrary
		p.DrewFromEmptyLibrary = false
		if p.Lost {
			continue
		}
		if drewFromEmpty || p.Life <= 0 || p.Counters.Count(Poison) >= 10 {
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
		// Actor is the winner, or NoPlayer for a draw -- there is no
		// separate PlayerLost event, so this is also where a loss is
		// visible in the stream: everyone else in the game lost.
		g.sink.Emit(Event{Kind: GameEnded, Active: g.activePlayer, Actor: remaining, Turn: uint16(g.turn)})
		return true
	}

	// CR 704.5q
	for _, pid := range g.Players() {
		for _, id := range g.Zone(Battlefield, pid).Cards() {
			annihilateCounters(g.Card(id))
		}
	}
	cleanupDanglingAttachments(g)
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

// cleanupDanglingAttachments is the one case CR 704.5f/704.5m checks that
// this port can decide without the layer system or a valid-string
// evaluator: the card an attachment pointed at left the battlefield out
// from under it. Move already unattaches a card from whatever *it* was
// attached to the moment it leaves (game.go); this is the other direction
// -- nothing walked the leaving card's own attachments -- and it has to be
// an SBA, not something Move does inline, because a Zone or Move test
// exercising a single card should not have to know about Aura at all.
//
// An Aura goes to its owner's graveyard whether the host left or the Aura
// was never attached to begin with -- both are "not attached to a legal
// object" (CR 704.5f). An Equipment or Fortification only loses the
// attachment, not the permanent (CR 704.5m): staying on the battlefield
// unattached is legal for those two, the way it is not for an Aura.
//
// Candidates are collected before either Move or Unattach runs, because
// both mutate the battlefield zone or a card's own attachment list -- the
// same hazard the zone-snapshot every zone read hands out already carries,
// just reachable here for the first time.
func cleanupDanglingAttachments(g *Game) {
	var toGraveyard, toUnattach []CardID
	for _, pid := range g.Players() {
		for _, id := range g.Zone(Battlefield, pid).Cards() {
			c := g.Card(id)
			host, attached := c.AttachedTo()
			legal := attached && g.Card(host).Zone == Battlefield
			switch {
			case legal:
				continue
			case c.Type().HasSubtype("Aura"):
				toGraveyard = append(toGraveyard, id)
			case attached:
				toUnattach = append(toUnattach, id)
			}
		}
	}
	for _, id := range toUnattach {
		g.Unattach(id)
	}
	for _, id := range toGraveyard {
		// Move unattaches id itself as a side effect of leaving the
		// battlefield (game.go), so there is nothing left to do here.
		g.Move(id, Graveyard, g.Card(id).Owner)
	}
}
