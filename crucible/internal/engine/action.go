// State-based actions and the game-over detection that rides along with
// them.

package engine

import "github.com/jczastkiewicz/crucible/internal/cardtype"

// CheckStateBasedActions applies every state-based action this port checks
// today and reports whether the game has ended as a result.
//
// Ported from forge-game/src/main/java/forge/game/GameAction.java
// (checkGameOverCondition, stateBasedAction704_5q, and the inline checks in
// its own checkStaticAbilities/state-based-action loop) and
// forge/game/player/Player.java (checkLoseCondition). CR numbers below are
// the ones Java's own comments cite, chased down line by line rather than
// assumed from the rulebook: GameAction.java labels the toughness check
// "Rule 704.5f", not 704.5g, and Forge's own comments disagree with each
// other about a couple of the others (noted where that happens) -- citing
// the wrong letter is worse than citing none, since a reader who goes
// looking for the Java source and finds a different rule at that letter
// has no way to tell whether the port or the citation is wrong.
//
// Ten of Java's checks are here: CR 704.5b (an attempted draw with
// nothing to draw loses), CR 704.5a (a player at zero or less life loses),
// CR 704.5c (ten or more poison counters loses), CR 704.5q (a permanent
// carrying both +1/+1 and -1/-1 counters loses the smaller pile from each,
// in equal number -- `stateBasedAction704_5q`'s own name is the source for
// this letter), a partial CR 704.5f (a creature at zero or less toughness
// -- printed, Layer 7's own SETPT/MODIFYPT/CHARACTERISTIC effects and +1/+1
// or -1/-1 counters all folded in, Card.Toughness's own job -- goes to its
// owner's graveyard), CR 704.5g and 704.5h together (a creature dealt
// damage at least equal to its toughness, or dealt any deathtouch damage
// at all, is destroyed -- destroyDamagedCreatures, below), a partial CR
// 704.5v (a Battle at zero or less defense goes to its owner's graveyard --
// destroyZeroDefense, below), and three rules Java's own comments do not
// cleanly single-letter: a partial "cleanup aura" (Java's own comment for
// it, GameAction.java:1511 -- an Aura not attached to anything on the
// battlefield goes to its owner's graveyard; an Equipment or Fortification
// attached to something no longer on the battlefield becomes unattached
// alongside it, folded into the same nearby but differently-labelled
// `stateBasedAction704_attach`), a planeswalker at zero loyalty
// (`handlePlaneswalkerRule`, which Java's own comments do not number at
// all), and the legend rule (`handleLegendRule`, same -- resolveLegendRule,
// below, is the first state-based action that needs a PlayerController, so
// CheckStateBasedActions takes one now). Every other rule in Java's loop --
// indestructible aside (destroyDamagedCreatures checks it; nothing else
// here needs to), the rest of 704.5f's own toughness (a "*" with no
// characteristic-defining effect to replace it, or a Count$ reference --
// `internal/expr` has no evaluator yet), the rest of 704.5v's own exception
// (a Battle whose own trigger is still on the stack -- always false today,
// destroyZeroDefense's own doc comment) and 704.5w/704.5x's protector
// assignment (needs combat and a new PlayerController decision, neither
// built), the rest of the attachment rules' own legality (an Aura's own
// "Enchant" restriction being violated by something other than its host
// leaving, protection, hexproof), and the legend rule's own two corner
// cases (resolveLegendRule's doc comment) -- needs either the rest of the
// continuous-effect layer system (type, color, ability layers; CR
// 613.6-613.8's dependency reordering, which nothing here has more than one
// effect to need yet), combat, a new decision point, or a valid-string
// evaluator to check a restriction this port does not have (game-state.md's
// "Not ported yet", `internal/valid`'s own doc comment). A rule this port
// has not implemented simply never fires, the same as it would in a real
// game with no permanent that rule applies to.
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
// can make another one true (destroying a permanent can, in turn, empty an
// Aura's target -- exactly the interaction 704.5f, 704.5g/704.5h, 704.5v, the
// legend rule and the attachment cleanup below have, which is why
// destroyLethalToughness, destroyDamagedCreatures, destroyZeroLoyalty,
// destroyZeroDefense and resolveLegendRule all run before
// cleanupDanglingAttachments rather than on a later call). Nothing here
// cascades a second time: destroying a permanent cannot itself change another
// one's printed toughness, deal it damage, or give it the same name, and
// nothing yet grants an effect that could. One pass is complete; the loop
// returns once a rule that can cascade twice lands.
//
// A game that has already ended skips every check below entirely, the same
// as Java: checkStateEffects returns as soon as checkGameOverCondition finds
// the game over, before its creature loop ever runs.
//
// A GameEnded event fires exactly once, on the call that flips g.over --
// never on a later call finding it already true, and not from any other
// path: this is the only place g.over is set.
func CheckStateBasedActions(g *Game, controller PlayerController) bool {
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
	destroyDamagedCreatures(g)
	destroyZeroLoyalty(g)
	destroyZeroDefense(g)
	resolveLegendRule(g, controller)
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

// destroyLethalToughness is CR 704.5f, GameAction.java's own comment (not
// 704.5g -- see CheckStateBasedActions's doc comment). Card.Toughness
// (card.go) folds Layer 7's continuous effects and +1/+1 and -1/-1
// counters onto the printed value already, so a creature reduced to zero
// by an annihilated -1/-1 pile, a MODIFYPT pump, or a SETPT/CHARACTERISTIC
// effect all die here the same as one whose printed toughness always read
// zero. What still does not die: a creature whose toughness is
// unresolvable at every layer -- "*" with no characteristic-defining
// effect to replace it, or a Count$ reference -- since Toughness reports
// that as ok=false rather than a wrong number (game-state.md's "Not
// ported yet").
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

// destroyDamagedCreatures is CR 704.5g and 704.5h together, GameAction.java's
// own comments (both cited on the same `else if`, since Java checks them in
// one branch): a creature dealt damage at least equal to its current
// toughness is destroyed, and a creature dealt any amount of deathtouch
// damage is destroyed regardless of the amount (CR 702.2c -- deathtouch
// makes any nonzero damage lethal on its own, so the marked total is never
// consulted for that half of the check). Indestructible
// (`c.hasKeyword(Keyword.INDESTRUCTIBLE)` in Java, an earlier branch in the
// same if/else chain) skips both: an indestructible creature that has taken
// lethal damage is not destroyed here, the one keyword this port checks
// anywhere, because getting it wrong would make this SBA actively incorrect
// for those cards rather than merely incomplete.
//
// Card.Toughness already folds Layer 7 and +1/+1/-1/-1 counters onto the
// printed value (`## Layer 7`, game-state.md), so this reads the same
// current toughness destroyLethalToughness does; a creature whose toughness
// is unresolvable at every layer is left alone here too, for the same
// reason.
//
// Candidates are collected before Move runs, the same reason every other
// SBA in this file does.
func destroyDamagedCreatures(g *Game) {
	var dead []CardID
	for _, pid := range g.Players() {
		for _, id := range g.Zone(Battlefield, pid).Cards() {
			c := g.Card(id)
			if !c.Type().Has(cardtype.Creature) || c.HasKeyword("Indestructible") {
				continue
			}
			if c.Damage.Deathtouch {
				dead = append(dead, id)
				continue
			}
			if t, ok := c.Toughness(); ok && c.Damage.Marked > 0 && c.Damage.Marked >= t {
				dead = append(dead, id)
			}
		}
	}
	for _, id := range dead {
		g.Move(id, Graveyard, g.Card(id).Owner)
	}
}

// destroyZeroLoyalty is CR 704.5's planeswalker-loyalty rule -- Java's own
// GameAction.java does not cite a letter for handlePlaneswalkerRule
// (CheckStateBasedActions's doc comment), so none is asserted here either:
// a planeswalker with loyalty zero or less goes to its owner's graveyard.
// Loyalty is entirely counter-based
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

// destroyZeroDefense is CR 704.5v: a Battle at defense zero or less goes to
// its owner's graveyard, unless it is the source of a triggered ability
// that has triggered but not yet left the stack (`hasSourceOnStack` in
// Java) -- CR 704.5v's own exception exists so a Battle's own "when this
// reaches 0 defense" trigger still gets to resolve. This port checks the
// exception exactly, not by skipping it: `g.StackTop`'s kind of lookup
// would need to inspect every item, not just the top, since anything could
// be pushed above the Battle's own trigger by the time this runs, and CR
// 613.6-613.8's ordering makes "is it still there" the only question that
// matters. Today it is always answered no -- nothing puts a trigger on the
// stack yet (`## Stack`), so every Battle is checked as if the exception
// never applies, which is the exception's own correct answer whenever it
// genuinely does not.
//
// Defense, like Loyalty, is entirely counter-based (Card.BaseDefense's own
// doc comment): entering the battlefield with printed-defense-many Defense
// counters is CR 704.5v's own prerequisite, and this port has no ETB hook
// for that yet either (destroyZeroLoyalty's own doc comment, same gap).
//
// Not here: CR 704.5w/704.5x, a Battle's protector assignment. Both need
// combat (to know whether the Battle is currently being attacked) and,
// for a Siege, a new PlayerController decision ("choose an opponent to
// protect this battle") this port has not built. A Battle with no
// protector assigned is not itself destroyed by that gap -- only
// zero-or-less Defense destroys a Battle -- so this SBA is correct on its
// own terms even without the other half existing yet.
func destroyZeroDefense(g *Game) {
	var dead []CardID
	for _, pid := range g.Players() {
		for _, id := range g.Zone(Battlefield, pid).Cards() {
			c := g.Card(id)
			if c.Type().Has(cardtype.Battle) && c.Counters.Count(Defense) <= 0 {
				dead = append(dead, id)
			}
		}
	}
	for _, id := range dead {
		g.Move(id, Graveyard, g.Card(id).Owner)
	}
}

// resolveLegendRule is the legend rule -- another of Java's own comments do
// not number (`handleLegendRule`, CheckStateBasedActions's doc comment): a
// player controlling two or more legendary permanents that share a name
// keeps one and puts the rest into their owners' graveyards.
//
// Grouping is per player, not across the whole battlefield: two different
// players may each legally control their own copy of one legendary
// permanent, so only a player's own duplicates trigger this. Within a
// player, names are grouped in the order their permanents first appear on
// the battlefield (GO-12) -- the same determinism `Multimaps.index`'s
// insertion-ordered keys give Java.
//
// Two of Java's own corner cases are not here: a legendary permanent that
// opts out via `ignoreLegendRule` (nothing this port can grant that effect
// yet), and Partner-with-a-non-legendary-creature-name pairs (Spy Kit and
// similar) sharing a "true name" even though their printed names differ --
// a rule specific to a handful of cards, not the general case.
func resolveLegendRule(g *Game, controller PlayerController) {
	for _, pid := range g.Players() {
		byName := map[string][]CardID{}
		var order []string
		for _, id := range g.Zone(Battlefield, pid).Cards() {
			c := g.Card(id)
			if !c.Type().HasSupertype(cardtype.Legendary) {
				continue
			}
			name := c.Def.Name
			if _, ok := byName[name]; !ok {
				order = append(order, name)
			}
			byName[name] = append(byName[name], id)
		}
		for _, name := range order {
			dup := byName[name]
			if len(dup) < 2 {
				continue
			}
			keep := controller.ChooseLegendaryToKeep(g, pid, dup)
			for _, id := range dup {
				if id != keep {
					g.Move(id, Graveyard, g.Card(id).Owner)
				}
			}
		}
	}
}

// cleanupDanglingAttachments is CR 704.5's attachment-legality rule, which
// this port can decide without the layer system or a valid-string
// evaluator only for the one case where the card an attachment pointed at
// left the battlefield out from under it. Not one clean letter: Java's own
// "cleanup aura" comment (GameAction.java:1511, CheckStateBasedActions's
// doc comment) is unlabeled, and the nearby attach-legality check it
// shares a loop with is labeled 704.5q in one comment even though
// `stateBasedAction704_5q`'s own name gives that letter to counter
// annihilation instead -- Java's comments disagree with each other here,
// so no sub-letter is asserted for this rule either. Move already
// unattaches a card from whatever *it* was attached to the moment it
// leaves (game.go); this is the other direction -- nothing walked the
// leaving card's own attachments -- and it has to be an SBA, not something
// Move does inline, because a Zone or Move test exercising a single card
// should not have to know about Aura at all.
//
// An Aura goes to its owner's graveyard whether the host left or the Aura
// was never attached to begin with -- both are "not attached to a legal
// object". An Equipment or Fortification only loses the attachment, not
// the permanent: staying on the battlefield unattached is legal for those
// two, the way it is not for an Aura.
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
