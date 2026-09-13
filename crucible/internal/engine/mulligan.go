// Mulligans: the pre-game procedure for replacing an opening hand.

package engine

// startingHandSize is CR 103.4's seven. Nothing that could change it --
// Font of Mythos, Vancouver's own scry step, a starting-hand-size static
// ability -- is built yet, so it is a constant rather than a Player field
// invented ahead of a caller that would set it.
const startingHandSize = 7

// PerformMulligans runs the London mulligan procedure for every seated
// player, ported from forge-game/src/main/java/forge/game/mulligan/
// MulliganService.java and LondonMulligan.java.
//
// London is the only rule ported. It is the one modern paper Magic has used
// since 2019 and the only one relevant to the constructed corpus Crucible
// targets (ADR-0011); Original, Paris and Vancouver are the rules London
// replaced, and Houston is a Forge-specific casual variant. Porting a
// strategy hierarchy for four rules nothing in scope exercises would be
// exactly the speculative work CLAUDE.md rules out (PORT-6).
//
// Callers deal each player's opening hand before calling this -- Java's
// MulliganService assumes Game already has, and dealing one needs a Match or
// StartGame flow this port has not built.
func PerformMulligans(g *Game, controller PlayerController, firstPlayer PlayerID) {
	order := turnOrderFrom(g, firstPlayer)
	// CR 103.4: every player gets one free mulligan in a game with more than
	// two players. Brawl grants the same and is not modeled.
	freeFirst := len(order) > 2

	kept := make(map[PlayerID]bool, len(order))
	timesMulliganed := make(map[PlayerID]int, len(order))

	for {
		allKept := true
		for _, pid := range order {
			if kept[pid] {
				continue
			}
			tuck := londonTuckCount(timesMulliganed[pid], freeFirst)
			keep := tuck > startingHandSize || controller.MulliganKeepHand(g, pid, firstPlayer, tuck)
			if g.Over() {
				return
			}
			if keep {
				kept[pid] = true
				continue
			}
			allKept = false
			mulligan(g, controller, pid, freeFirst, &timesMulliganed)
		}
		if allKept {
			break
		}
	}
}

// mulligan is one London mulligan: shuffle the hand back into the library,
// draw a fresh seven, then bottom the cards this mulligan costs. Ported from
// AbstractMulligan.mulligan and LondonMulligan.mulliganDraw.
func mulligan(g *Game, controller PlayerController, pid PlayerID, freeFirst bool, timesMulliganed *map[PlayerID]int) {
	for _, id := range append([]CardID(nil), g.Zone(Hand, pid).Cards()...) {
		g.Move(id, Library, pid)
	}
	g.Shuffle(Library, pid)
	(*timesMulliganed)[pid]++

	lib := g.Zone(Library, pid)
	for i := 0; i < startingHandSize && lib.Len() > 0; i++ {
		g.Move(lib.Cards()[0], Hand, pid)
	}

	tuck := londonTuckCount((*timesMulliganed)[pid], freeFirst)
	if tuck == 0 {
		return
	}
	hand := append([]CardID(nil), g.Zone(Hand, pid).Cards()...)
	// canMulligan's cutoff (tuck <= startingHandSize) is checked against the
	// PREVIOUS mulligan count, one step behind what this one actually costs
	// -- Java's own LondonMulligan.canMulligan reads the same field
	// tuckCardsDuringMulligan does, before this mulligan's increment. That
	// lets the last offered mulligan cost one more card than a hand can
	// give back; clamping here is a defensive floor against asking a
	// PlayerController for more cards than exist, not a rules change --
	// tucking every remaining card and tucking "every remaining card, and
	// then some" both leave an empty hand.
	if tuck > len(hand) {
		tuck = len(hand)
	}
	for _, id := range controller.TuckCardsViaMulligan(g, pid, hand, tuck) {
		g.Move(id, Library, pid)
	}
}

// londonTuckCount is LondonMulligan.tuckCardsDuringMulligan: no cost for the
// hand you kept without ever mulliganing, and one mulligan is free in a
// multiplayer game.
func londonTuckCount(timesMulliganed int, freeFirst bool) int {
	if timesMulliganed == 0 {
		return 0
	}
	if freeFirst {
		return timesMulliganed - 1
	}
	return timesMulliganed
}

// turnOrderFrom is seating order starting at first, the same rotation
// MulliganService.initializeMulligans builds before asking anyone anything.
func turnOrderFrom(g *Game, first PlayerID) []PlayerID {
	ids := g.Players()
	start := 0
	for i, id := range ids {
		if id == first {
			start = i
			break
		}
	}
	out := make([]PlayerID, len(ids))
	for i := range ids {
		out[i] = ids[(start+i)%len(ids)]
	}
	return out
}
