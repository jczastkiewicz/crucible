// The turn structure: which phase follows which, whose turn it is, and the
// per-step actions this port has reached.
//
// Ported from forge-game/src/main/java/forge/game/phase/PhaseHandler.java.
// Only two steps have a body: Untap and Draw. Every other step in
// PhaseHandler.onPhaseBegin needs the stack, triggers, SpellAbility or
// Combat to do anything -- upkeep triggers, casting in a main phase,
// declaring attackers -- so AdvancePhase walks through them as bookkeeping
// only, changing ActivePhase and nothing else, until each one's turn comes
// (porting/port-log/game-state.md).
//
// PhaseHandler's priority loop (mainLoopStep) is not here either. With no
// stack and no PlayerController method that can cast anything, priority is
// vacuous today: a player asked "do you have a legal action" always answers
// no, so the loop would exist only to immediately fall through every time.
// Building that as a stub would be scaffolding for a decision no one can
// make yet -- it lands with the stack (M5, later).

package engine

// SetTurnState jumps the game directly to a turn, active player and phase,
// with none of the per-step actions applied. This is fixture loading's tool
// -- the same relationship Java's devAdvanceToPhase has to
// advanceToNextPhase: a state injection for tests, not a play action. Real
// play reaches a turn state through StartTurn and AdvancePhase only.
func (g *Game) SetTurnState(turn int, active PlayerID, phase PhaseType) {
	g.turn, g.activePlayer, g.activePhase = turn, active, phase
}

// StartTurn begins the game's first turn -- turn 1, phase Untap, active as
// given -- and runs the untap step's actions and the state-based-action
// check that follows every phase entry. Ported from
// PhaseHandler.startFirstTurn.
func (g *Game) StartTurn(active PlayerID) {
	g.turn = 1
	g.activePlayer = active
	g.activePhase = Untap
	g.sink.Emit(Event{Kind: TurnBegan, Active: active, Turn: uint16(g.turn)})
	g.beginPhase()
}

// AdvancePhase moves to the next step or phase, rotating the active player
// and incrementing Turn when one wraps past Cleanup back to Untap. It runs
// that phase's actions and the state-based-action check that follows,
// ported from PhaseHandler.advanceToNextPhase and onPhaseBegin.
//
// Java's extra-turn and extra-phase stacks (AddTurnEffect, SkipPhaseEffect)
// and its topsy-turvy phase order (a handful of effects that reverse it) are
// not here: both are abilities this port has not implemented, so nothing can
// push onto either yet, and skipping them is not a gap a card can currently
// expose.
func (g *Game) AdvancePhase() {
	next := PhaseType((int(g.activePhase) + 1) % numPhaseTypes)
	if next == Untap {
		g.turn++
		g.activePlayer = g.nextPlayerAfter(g.activePlayer)
		g.sink.Emit(Event{Kind: TurnBegan, Active: g.activePlayer, Turn: uint16(g.turn)})
	}
	g.activePhase = next
	g.beginPhase()
}

// nextPlayerAfter is turn order: seating order, skipping anyone who has
// lost (CR 800-something -- a player who has left the game is skipped when
// play passes to them). A game that reaches here with everyone else lost is
// already over; returning p unchanged rather than panicking is safe because
// nothing calls AdvancePhase without checking Over first once that happens.
func (g *Game) nextPlayerAfter(p PlayerID) PlayerID {
	ids := g.Players()
	start := 0
	for i, id := range ids {
		if id == p {
			start = i
			break
		}
	}
	for i := 1; i <= len(ids); i++ {
		next := ids[(start+i)%len(ids)]
		if !g.Player(next).Lost {
			return next
		}
	}
	return p
}

// beginPhase runs the active phase's turn-based actions, then the
// state-based-action check CR 704.3 requires before anyone can act -- the
// same pairing Java's onPhaseBegin and checkStateBasedEffects run back to
// back at the top of mainLoopStep.
func (g *Game) beginPhase() {
	g.sink.Emit(Event{Kind: PhaseBegan, Phase: g.activePhase, Active: g.activePlayer, Turn: uint16(g.turn)})
	switch g.activePhase {
	case Untap:
		g.untapStep()
	case Draw:
		g.drawStep()
	}
	CheckStateBasedActions(g)
}

// untapStep untaps every permanent the active player controls and clears
// their summoning sickness (CR 302.6): a permanent still on the battlefield
// at its controller's own untap step has, by definition, been controlled
// continuously since their most recent turn began.
//
// Effects that skip a permanent's untap (CR 502.3) are not modeled -- no
// card can grant that yet -- so every permanent the active player controls
// untaps unconditionally.
func (g *Game) untapStep() {
	for _, id := range g.Zone(Battlefield, g.activePlayer).Cards() {
		c := g.Card(id)
		c.Tapped = false
		c.SummonSick = false
	}
}

// drawStep draws one card for the active player, ported from
// PhaseHandler.onPhaseBegin's DRAW case and PhaseHandler.isSkippingPhase's
// DRAW rule (CR 103.7a): the first player skips the draw step of their own
// first turn in a two-player game. A library with nothing left to draw
// records the attempt rather than silently doing nothing -- CheckStateBasedActions
// reads that flag for CR 704.5b.
//
// The top of the library is index 0 of the zone's order: a fixture author
// who writes `humanlibrary=TopCard;NextCard;...` names it left to right, top
// to bottom, and Load builds cards in that same order (game-state-fixture.md).
func (g *Game) drawStep() {
	if g.turn == 1 && len(g.Players()) == 2 {
		return
	}
	lib := g.Zone(Library, g.activePlayer)
	if lib.Len() == 0 {
		g.Player(g.activePlayer).DrewFromEmptyLibrary = true
		return
	}
	id := lib.Cards()[0]
	g.Move(id, Hand, g.activePlayer)
	// CardDrawn alongside the ZoneChanged Move already emitted: ZoneChanged
	// says a card moved, CardDrawn says why, which is what makes a draw
	// countable without inspecting every zone change for the ones that
	// happen to be library-to-hand.
	g.sink.Emit(Event{Kind: CardDrawn, Phase: g.activePhase, Active: g.activePlayer, Actor: g.activePlayer, Turn: uint16(g.turn), Source: id})
}
