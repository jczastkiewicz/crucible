// The turn structure: which phase follows which, whose turn it is, and the
// per-step actions this port has reached.
//
// Ported from forge-game/src/main/java/forge/game/phase/PhaseHandler.java.
// Four steps have a body: Untap, Draw, CombatEnd (CR 511.3's "remove every
// creature and planeswalker/battle from combat" -- pure bookkeeping this
// port already has everything it needs for, unlike the rest of Combat) and
// Cleanup (CR 514.1's discard to hand size and CR 514.2's damage clear --
// 514.2's other half, ending "until end of turn" effects, still needs
// machinery this port has not reached, below). Every step, not only those
// four, now checks CR 500's own "at the beginning of a step or phase"
// trigger (checkPhaseTriggers, trigger.go) right after its own body, if any
// -- casting in a main phase and declaring attackers still wait on the rest
// of Combat/the stack's real priority loop, so AdvancePhase still walks
// through the steps that need those as bookkeeping only, changing
// ActivePhase and nothing else beyond the trigger check, until each one's
// turn comes (porting/port-log/game-state.md).
//
// PhaseHandler's priority loop (mainLoopStep) is still not wired in here,
// even though the stack itself now exists (stack.go). No PlayerController
// method can cast or activate anything, so a player asked "do you have a
// legal action" always answers no -- calling ResolveStack from beginPhase
// today would be a no-op on every call, since nothing yet pushes an ability
// in production. It lands here once something does (triggers, at the
// earliest).

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
//
// controller is CheckStateBasedActions's own (action.go): the legend rule
// is a state-based action that can fire on any phase entry, not just when a
// card is cast, so every path into beginPhase needs one to hand it.
func (g *Game) StartTurn(active PlayerID, controller PlayerController) {
	g.turn = 1
	g.activePlayer = active
	g.activePhase = Untap
	g.sink.Emit(Event{Kind: TurnBegan, Active: active, Turn: uint16(g.turn)})
	g.beginPhase(controller)
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
func (g *Game) AdvancePhase(controller PlayerController) {
	next := PhaseType((int(g.activePhase) + 1) % numPhaseTypes)
	if next == Untap {
		g.turn++
		g.activePlayer = g.nextPlayerAfter(g.activePlayer)
		g.sink.Emit(Event{Kind: TurnBegan, Active: g.activePlayer, Turn: uint16(g.turn)})
	}
	g.activePhase = next
	g.beginPhase(controller)
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
func (g *Game) beginPhase(controller PlayerController) {
	g.emptyManaPools()
	g.sink.Emit(Event{Kind: PhaseBegan, Phase: g.activePhase, Active: g.activePlayer, Turn: uint16(g.turn)})
	switch g.activePhase {
	case Untap:
		g.untapStep()
	case Draw:
		g.drawStep()
	case CombatEnd:
		g.endCombat()
	case Cleanup:
		g.cleanupStep(controller)
	}
	g.checkPhaseTriggers(controller)
	CheckStateBasedActions(g, controller)
}

// emptyManaPools is CR 500.4: as a step or phase ends, every player's
// floating mana empties, not just the active player's. Java runs the
// equivalent (PhaseHandler.onPhaseEnd, which calls Player.getManaPool().
// clearPool for every player) once per transition, right before the next
// phase begins; this port has no separate "phase ended" hook, so it runs at
// the top of beginPhase instead -- the same transition, the same "once per
// step or phase" cadence, just named for where this port's phase walk
// actually stops to do work (this file's own doc comment: "AdvancePhase
// walks through them as bookkeeping only... until each one's turn comes").
//
// Mana burn -- losing life for unspent mana -- is not reproduced: it left
// the rules in 2010, before any Standard-legal card this port's corpus
// targets was printed, so there is nothing to carry parity with.
func (g *Game) emptyManaPools() {
	for _, id := range g.Players() {
		g.Player(id).ManaPool.Empty()
	}
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
// first turn in a two-player game.
func (g *Game) drawStep() {
	if g.turn == 1 && len(g.Players()) == 2 {
		return
	}
	g.DrawCards(g.activePlayer, 1)
}

// DrawCards draws n cards for pid, one at a time (Player.drawCards' own
// per-card loop in Java, not a single Move of n cards at once) -- CR 120.3's
// "draw a card," repeated, matters once something reacts to an individual
// draw rather than the batch (nothing does yet, game-state.md's "Not ported
// yet"), so this port matches the granularity rather than guessing it never
// matters. A library that runs out partway through records the attempt
// (CheckStateBasedActions' own CR 704.5b) and stops -- the remaining draws
// never happened, the same as a real player who cannot pay to keep drawing
// past empty.
//
// The top of the library is index 0 of the zone's order: a fixture author
// who writes `humanlibrary=TopCard;NextCard;...` names it left to right, top
// to bottom, and Load builds cards in that same order (game-state-fixture.md).
// drawStep (above) and drawEffect (draweffect.go, CR 120.3/M6's own Draw
// effect) are this port's two callers.
func (g *Game) DrawCards(pid PlayerID, n int) {
	for i := 0; i < n; i++ {
		lib := g.Zone(Library, pid)
		if lib.Len() == 0 {
			g.Player(pid).DrewFromEmptyLibrary = true
			return
		}
		id := lib.Cards()[0]
		g.Move(id, Hand, pid)
		// CardDrawn alongside the ZoneChanged Move already emitted: ZoneChanged
		// says a card moved, CardDrawn says why, which is what makes a draw
		// countable without inspecting every zone change for the ones that
		// happen to be library-to-hand.
		g.sink.Emit(Event{Kind: CardDrawn, Phase: g.activePhase, Active: g.activePlayer, Actor: pid, Turn: uint16(g.turn), Source: id})
	}
}

// MaxHandSize is CR 103.4's default maximum hand size, the base
// HandSizeLimit (player.go) folds Layer 8's own SetMaxHandSize$/
// RaiseMaxHandSize$ continuous effects on top of (Spellbook's/Thought
// Vessel's own static abilities among the corpus's 43 real
// SetMaxHandSize$ lines).
const MaxHandSize = 7

// endCombat is CR 511.3: at the beginning of the end of combat step, every
// creature and planeswalker/battle is removed from combat. Java's
// PhaseHandler.endCombat sets its Combat field to null; this port's
// `Game.combat` is a value, not a pointer, so the zero value is the
// equivalent -- a fresh `Combat{}` has no Attackers, AttackTargets or
// Blocks left over.
//
// Real, unconditional bookkeeping, unlike every other step this file still
// walks past empty-handed: nothing here needs the stack, triggers,
// SpellAbility or more of Combat than already exists, so it does not wait
// on M6 the way Upkeep's or Main1's real bodies do. Before this, a turn
// that reached combat once and then had nothing eligible to attack with on
// a later turn would still report the earlier combat's Attackers and
// Blocks -- DeclareCombatAttackers/DeclareCombatBlockers only overwrite
// `g.combat` on the branch where something is actually declared, and
// neither returns early by clearing it (game-state.md's "Combat" section).
func (g *Game) endCombat() {
	g.combat = Combat{}
}

// cleanupStep is CR 514.1 (discard to maximum hand size) followed by a
// partial CR 514.2 ("all damage marked on permanents ... is removed") and
// CR 305.2's own per-turn land-play reset. 514.1 only concerns the active
// player -- discarding down is not scoped to everyone the way clearing
// damage and the land-play reset are (below); an untapStep-shaped difference
// these have from each other. If the active player's hand already fits, or
// is empty, the controller is never asked, the same "nothing meaningful to
// decide" reasoning every other combat/mulligan decision point in this port
// uses for an empty or already-satisfied set.
//
// Each discard checks CR 603's own "whenever ~ is discarded" trigger
// (checkDiscardedTriggers, trigger.go) right after Move, the same
// after-the-fact timing checkDiesTriggers already uses for a card that just
// left the battlefield.
//
// Not here: "until end of turn"/"this turn" effects ending (CR 514.2's
// other half, needs duration tracking this port does not have -- PT's own
// effects, for one, have no timestamp-scoped-to-a-turn concept yet,
// game-state.md's "Not ported yet"). Cleanup normally does not check
// state-based actions or allow priority at all (CR 514.3) unless a discard
// or an ending effect triggered something; since triggers aren't built and
// ending effects aren't tracked, that exception cannot fire either, so
// beginPhase's own CheckStateBasedActions call after this is technically
// one PhaseHandler does not make here -- harmless today, since nothing this
// port can do inside cleanupStep creates a new state-based condition to
// check for the first time in this same phase.
func (g *Game) cleanupStep(controller PlayerController) {
	hand := g.Zone(Hand, g.activePlayer).Cards()
	if limit, hasLimit := g.Player(g.activePlayer).HandSizeLimit(MaxHandSize); hasLimit && len(hand) > limit {
		discard := controller.DiscardToHandSize(g, g.activePlayer, hand, len(hand)-limit)
		for _, id := range discard {
			g.Move(id, Graveyard, g.Card(id).Owner)
			g.checkDiscardedTriggers(id, g.activePlayer)
		}
	}

	for _, pid := range g.Players() {
		for _, id := range g.Zone(Battlefield, pid).Cards() {
			g.Card(id).Damage.Clear()
		}
		p := g.Player(pid)
		p.LandsPlayedLastTurn = p.LandsPlayed
		p.LandsPlayed = 0
	}
}
