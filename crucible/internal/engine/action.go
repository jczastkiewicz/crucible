// State-based actions and the game-over detection that rides along with
// them.

package engine

import "github.com/jczastkiewicz/crucible/internal/cardtype"

// CheckStateBasedActions applies every state-based action this port checks
// today and reports whether the game has ended as a result.
//
// Ported from forge-game/src/main/java/forge/game/GameAction.java
// (checkGameOverCondition, stateBasedAction704_5q) and
// forge/game/player/Player.java (checkLoseCondition). Seven of Java's checks
// are here: CR 704.5b (an attempted draw with nothing to draw loses), CR
// 704.5a (a player at zero or less life loses), CR 704.5c (ten or more
// poison counters loses), CR 704.5q (a permanent carrying both +1/+1 and
// -1/-1 counters loses the smaller pile from each, in equal number), a
// partial CR 704.5g (a creature at zero or less toughness -- printed, Layer
// 7's own SETPT/MODIFYPT/CHARACTERISTIC effects and +1/+1 or -1/-1 counters
// all folded in, Card.Toughness's own job -- goes to its owner's graveyard),
// and a partial CR 704.5f/704.5m (an Aura not attached to anything on the
// battlefield goes to its owner's graveyard; an Equipment or Fortification
// attached to something no longer on the battlefield becomes unattached).
// Every other rule in Java's loop -- lethal damage, a planeswalker at zero
// loyalty, the rest of 704.5g's own toughness (a "*" with no
// characteristic-defining effect to replace it, or a Count$ reference --
// `internal/expr` has no evaluator yet), and the rest of 704.5f/704.5m's own
// legality (an Aura's own "Enchant" restriction being violated by something
// other than its host leaving, protection, hexproof) -- needs either the
// rest of the continuous-effect layer system (type, color, ability layers;
// CR 613.6-613.8's dependency reordering, which nothing here has more than
// one effect to need yet) or a permanent type (Planeswalker, Battle) this
// port has not built, or a valid-string evaluator to check a restriction
// this port does not have (game-state.md's "Not ported yet", `internal/valid`'s
// own doc comment). A rule this port has not implemented simply never
// fires, the same as it would in a real game with no permanent that rule
// applies to.
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
// Aura's target -- exactly the interaction 704.5g and the attachment cleanup
// below have, which is why destroyLethalToughness runs before
// cleanupDanglingAttachments rather than on a later call). Nothing here
// cascades a second time: destroying a creature cannot itself change another
// creature's printed toughness, and nothing yet grants an effect that could.
// One pass is complete; the loop returns once a rule that can cascade twice
// lands.
//
// A game that has already ended skips every check below entirely, the same
// as Java: checkStateEffects returns as soon as checkGameOverCondition finds
// the game over, before its creature loop ever runs.
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
	destroyLethalToughness(g)
	destroyZeroLoyalty(g)
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

// destroyLethalToughness is CR 704.5g: Card.Toughness (card.go) folds
// Layer 7's continuous effects and +1/+1 and -1/-1 counters onto the
// printed value already, so a creature reduced to zero by an annihilated
// -1/-1 pile, a MODIFYPT pump, or a SETPT/CHARACTERISTIC effect all die
// here the same as one whose printed toughness always read zero. What
// still does not die: a creature whose toughness is unresolvable at every
// layer -- "*" with no characteristic-defining effect to replace it, or a
// Count$ reference -- since Toughness reports that as ok=false rather than
// a wrong number (game-state.md's "Not ported yet").
//
// Candidates are collected before Move runs, the same reason
// cleanupDanglingAttachments collects first: Move mutates the battlefield
// zone this ranges over.
func destroyLethalToughness(g *Game) {
	var dead []CardID
	for _, pid := range g.Players() {
		for _, id := range g.Zone(Battlefield, pid).Cards() {
			c := g.Card(id)
			if !c.Type().Has(cardtype.Creature) {
				continue
			}
			if t, ok := c.Toughness(); ok && t <= 0 {
				dead = append(dead, id)
			}
		}
	}
	for _, id := range dead {
		g.Move(id, Graveyard, g.Card(id).Owner)
	}
}

// destroyZeroLoyalty is CR 704.5h: a planeswalker with loyalty zero or
// less goes to its owner's graveyard. Loyalty is entirely counter-based
// (Card.BaseLoyalty's own doc comment) -- there is no Layer 7 to fold, no
// printed-value fallback the way BaseToughness has one, so this reads
// Card.Counters.Count(Loyalty) directly rather than calling a "current
// loyalty" accessor that would just be that same call one level removed.
//
// Nothing yet puts a starting loyalty counter on a planeswalker when it
// enters the battlefield (CR 121.5): Move has no ETB hook for any
// permanent's starting counters today, the same gap "Move carries what
// Java gets for free" already documents for triggers and replacement
// effects. A fixture or a test sets Loyalty counters directly until that
// lands -- this SBA is real and correct against whatever count is there,
// however it got there.
//
// Candidates are collected before Move runs, the same reason
// destroyLethalToughness and cleanupDanglingAttachments do.
func destroyZeroLoyalty(g *Game) {
	var dead []CardID
	for _, pid := range g.Players() {
		for _, id := range g.Zone(Battlefield, pid).Cards() {
			c := g.Card(id)
			if c.Type().Has(cardtype.Planeswalker) && c.Counters.Count(Loyalty) <= 0 {
				dead = append(dead, id)
			}
		}
	}
	for _, id := range dead {
		g.Move(id, Graveyard, g.Card(id).Owner)
	}
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
