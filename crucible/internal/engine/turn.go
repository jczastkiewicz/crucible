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
// The next active player comes from nextActivePlayer: Java's extra-turn
// stack and its skipped turns. A phase an AddPhase effect queued after the
// current one comes first (g.extraPhases); a phase a SkipPhase effect names
// for the active player is passed over without beginning (consumeSkip).
func (g *Game) AdvancePhase(controller PlayerController) {
	var next PhaseType
	if st := g.extraPhases[g.activePhase]; len(st) > 0 {
		next = st[len(st)-1]
		g.extraPhases[g.activePhase] = st[:len(st)-1]
	} else {
		next = PhaseType((int(g.activePhase) + 1) % numPhaseTypes)
		if next == Untap {
			g.turn++
			g.previousPlayer = g.activePlayer
			g.previousPlayerSpells = g.Player(g.activePlayer).SpellsCastThisTurn
			for _, pid := range g.Players() {
				g.Player(pid).SpellsCastThisTurn = 0
			}
			g.extraPhases = [numPhaseTypes][]PhaseType{}
			g.combatsThisTurn = 0
			g.activePlayer = g.nextActivePlayer()
			g.endDetains(g.activePlayer)
			g.endGoads(g.activePlayer)
			g.delayedTriggersOnNextTurn(g.activePlayer)
			g.activateCleanupDelayedTriggers()
			g.endEffectsAtTurnStart(g.activePlayer)
			g.monarchBeginTurn = g.monarch
			g.sink.Emit(Event{Kind: TurnBegan, Active: g.activePlayer, Turn: uint16(g.turn)})
		}
	}
	g.activePhase = next
	if g.consumeSkip(next) {
		// ReplaceBeginPhase replaced it: a skipped combat phase jumps to its
		// end step, then the phase walk carries on (advanceToNextPhase).
		if next == CombatBegin {
			g.activePhase = CombatEnd
		}
		g.AdvancePhase(controller)
		return
	}
	g.beginPhase(controller)
}

// nextActivePlayer is PhaseHandler.getNextActivePlayer: the top of the
// extra-turn stack when it has one, otherwise the player after the current
// one in turn order. A player with TurnsToSkip left loses that turn (Java's
// BeginTurn replacement, which counts down); a skipped normal turn still
// advances the turn-order cursor past them, and the pick repeats. An extra
// turn belonging to a player who has since lost is dropped.
func (g *Game) nextActivePlayer() PlayerID {
	cursor := g.activePlayer
	for guard := 0; guard < 1000; guard++ {
		var next PlayerID
		fromExtra := false
		if n := len(g.extraTurns); n > 0 {
			next, fromExtra = g.extraTurns[n-1], true
			g.extraTurns = g.extraTurns[:n-1]
		} else {
			next = g.nextPlayerAfter(cursor)
		}
		p := g.Player(next)
		if p.Lost {
			if !fromExtra {
				// nextPlayerAfter only returns a lost player when every
				// player has lost; the game is over and turn order is moot.
				return next
			}
			continue
		}
		if p.TurnsToSkip > 0 {
			p.TurnsToSkip--
			if !fromExtra {
				cursor = next
			}
			continue
		}
		return next
	}
	panic("engine: nextActivePlayer found no player able to take a turn")
}

// addExtraTurn is PhaseHandler.addExtraTurn: the first extra turn also
// pushes the player whose normal turn comes next, so turn order resumes
// from the right seat when the extra turns are done.
func (g *Game) addExtraTurn(p PlayerID) {
	if len(g.extraTurns) == 0 {
		g.extraTurns = append(g.extraTurns, g.nextPlayerAfter(g.activePlayer))
	}
	g.extraTurns = append(g.extraTurns, p)
}

// nextPlayerAfter is turn order: seating order, skipping anyone who has
// lost (CR 800-something -- a player who has left the game is skipped when
// play passes to them). A game that reaches here with everyone else lost is
// already over; returning p unchanged rather than panicking is safe because
// nothing calls AdvancePhase without checking Over first once that happens.
func (g *Game) nextPlayerAfter(p PlayerID) PlayerID {
	return g.nextPlayerInDirection(p, g.turnOrderReversed)
}

// nextPlayerInDirection is Game.getNextPlayerAfter(p, direction): the next
// player still in the game after p, walking the seats forwards (Direction.
// Left) or, with right, backwards.
func (g *Game) nextPlayerInDirection(p PlayerID, right bool) PlayerID {
	ids := g.Players()
	start := 0
	for i, id := range ids {
		if id == p {
			start = i
			break
		}
	}
	step := 1
	if right {
		step = len(ids) - 1
	}
	for i := 1; i <= len(ids); i++ {
		next := ids[(start+i*step)%len(ids)]
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
		g.untapStep(controller)
	case Draw:
		g.drawStep(controller)
	case CombatBegin:
		g.combatsThisTurn++
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
// CR 502.3/614.17's own "doesn't untap" replacement effects are checked
// per permanent (untapBlocked, replacement.go) before Tapped is cleared --
// summoning sickness clears regardless, since a "doesn't untap" effect
// restricts only the untapping action, not CR 302.6's own continuous-control
// question. CR 701.42b's own exert check runs first, Card.untap(phase)'s own
// "if (phase != null && isExertedBy(phase)) return false" ordering ported
// directly -- an exerted permanent skips both the replacement-effect check
// and Mode$ Untaps entirely, the identical early return an already-untapped
// permanent gets. Exerted itself clears unconditionally every untap step
// regardless of whether untapping was actually skipped for it or any other
// reason (Untap.java's own separate "remove exerted flags from all things
// in play" pass, unconditional there too).
//
// checkUntapsTriggers (trigger.go) fires once per card that actually
// untaps -- Card.untap()'s own early "if (!tapped) return false", ported as
// the wasTapped check below rather than inside checkUntapsTriggers itself,
// since a card already untapped is not an event to check triggers against
// at all.
func (g *Game) untapStep(controller PlayerController) {
	g.dayTimeAtUntap()
	for _, id := range g.Zone(Battlefield, g.activePlayer).Cards() {
		c := g.Card(id)
		wasTapped := c.Tapped
		exerted := c.Exerted
		c.Exerted = false
		if !exerted && !g.untapBlocked(c) {
			c.Tapped = false
			if wasTapped {
				g.checkUntapsTriggers(controller, id)
			}
		}
		c.SummonSick = false
	}
}

// drawStep draws one card for the active player, ported from
// PhaseHandler.onPhaseBegin's DRAW case and PhaseHandler.isSkippingPhase's
// DRAW rule (CR 103.7a): the first player skips the draw step of their own
// first turn in a two-player game. Every player's own DrawnThisDrawStep
// (player.go) resets here first, PhaseHandler.java:268-271's own loop over
// every player -- a skipped draw step resets nothing, matching Java's own
// case DRAW body never running at all when the step itself is skipped.
func (g *Game) drawStep(controller PlayerController) {
	if g.turn == 1 && len(g.Players()) == 2 {
		return
	}
	for _, pid := range g.Players() {
		g.Player(pid).DrawnThisDrawStep = 0
	}
	g.DrawCards(g.activePlayer, 1, controller)
}

// DrawCards draws n cards for pid, one at a time (Player.drawCards' own
// per-card loop in Java, not a single Move of n cards at once) -- CR 120.3's
// "draw a card," repeated, matters once something reacts to an individual
// draw rather than the batch: checkDrawnTriggers (trigger.go) now does,
// firing once per card with that card's own running CardsDrawnThisTurn
// count, the reason this port matches the granularity rather than guessing
// it never matters. A library that runs out partway through records the attempt
// (CheckStateBasedActions' own CR 704.5b) and stops -- the remaining draws
// never happened, the same as a real player who cannot pay to keep drawing
// past empty.
//
// drawPrevented (replacement.go) is checked first, one card at a time --
// Player.doDraw's own Event$ Draw replacement check running before it ever
// looks at whether the library is empty, ported directly: a card this
// checks true for never reaches the empty-library check below at all, so a
// draw CR 614 prevents cannot also be the "attempted to draw from an empty
// library" 704.5b loses to. drawReplaced (replacement.go, new) is checked
// next, the identical "before the empty-library check" ordering -- CR 616's
// own "the event is replaced by a different one" outcome, distinct from
// Prevent$'s "the event does not happen at all": possessed_portal.txt's own
// bare Prevent$ line and thought_reflection.txt's own bare ReplaceWith$ line
// both skip this card's own normal draw below, for two different reasons.
//
// The top of the library is index 0 of the zone's order: a fixture author
// who writes `humanlibrary=TopCard;NextCard;...` names it left to right, top
// to bottom, and Load builds cards in that same order (game-state-fixture.md).
// drawStep (above) and drawEffect (draweffect.go, CR 120.3/M6's own Draw
// effect) are this port's two callers.
func (g *Game) DrawCards(pid PlayerID, n int, controller PlayerController) {
	for i := 0; i < n; i++ {
		if g.drawPrevented(pid) {
			continue
		}
		if g.drawReplaced(controller, pid) {
			continue
		}
		if !g.drawOneCard(controller, pid) {
			return
		}
	}
}

// drawOneCard is CR 120.3's own primitive: move the top card of pid's
// library to pid's hand, or record CR 704.5b's own "attempted to draw from
// an empty library" and report false. DrawCards' own per-card loop and
// drawReplaced's own ReplaceWith$-to-Draw dispatch (replacement.go) both
// call this rather than duplicating it -- a replacement's own "draw two
// cards instead" is CR 120.3's identical primitive run twice, not a
// different action.
func (g *Game) drawOneCard(controller PlayerController, pid PlayerID) bool {
	lib := g.Zone(Library, pid)
	if lib.Len() == 0 {
		g.Player(pid).DrewFromEmptyLibrary = true
		return false
	}
	id := lib.Cards()[0]
	g.Move(id, Hand, pid)
	// CardDrawn alongside the ZoneChanged Move already emitted: ZoneChanged
	// says a card moved, CardDrawn says why, which is what makes a draw
	// countable without inspecting every zone change for the ones that
	// happen to be library-to-hand.
	g.sink.Emit(Event{Kind: CardDrawn, Phase: g.activePhase, Active: g.activePlayer, Actor: pid, Turn: uint16(g.turn), Source: id})
	g.Player(pid).CardsDrawnThisTurn++
	if g.activePhase == Draw {
		g.Player(pid).DrawnThisDrawStep++
	}
	g.checkDrawnTriggers(controller, pid, id, g.Player(pid).CardsDrawnThisTurn)
	return true
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
	g.endEffectsAtEndOfCombat()
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
// CR 514.2's other half, "until end of turn"/"this turn" effects ending, is
// here too now: every non-Permanent Pump effect (Game.pumps, game.go) is
// dropped, the way a resolved Giant Growth stops applying once its own turn
// ends. Cleanup normally does not check state-based actions or allow
// priority at all (CR 514.3) unless a discard or an ending effect triggered
// something; a Pump wearing off triggers nothing this port has any
// Mode$ for, so beginPhase's own CheckStateBasedActions call after this is
// technically one PhaseHandler does not make here -- harmless, since nothing
// this port can do inside cleanupStep creates a new state-based condition to
// check for the first time in this same phase.
func (g *Game) cleanupStep(controller PlayerController) {
	hand := g.Zone(Hand, g.activePlayer).Cards()
	if limit, hasLimit := g.Player(g.activePlayer).HandSizeLimit(MaxHandSize); hasLimit && len(hand) > limit {
		discard := controller.DiscardToHandSize(g, g.activePlayer, hand, len(hand)-limit)
		for _, id := range discard {
			g.Move(id, Graveyard, g.Card(id).Owner)
			g.checkDiscardedTriggers(controller, id, g.activePlayer)
		}
	}

	for _, pid := range g.Players() {
		for _, id := range g.Zone(Battlefield, pid).Cards() {
			g.Card(id).Damage.Clear()
			g.Card(id).AttacksThisTurn = 0
			g.Card(id).BecameTargetThisTurn = false
			g.Card(id).LoyaltyAbilityActivated = false
			g.Card(id).RegenShields = 0
		}
		p := g.Player(pid)
		p.LandsPlayedLastTurn = p.LandsPlayed
		p.LandsPlayed = 0
		p.CardsDrawnThisTurn = 0
		p.DescendedThisTurn = false
		p.LifeGainedTimesThisTurn = 0
	}

	g.combatDamagePrevented = false
	g.preventShields = nil

	kept := g.pumps[:0]
	for _, p := range g.pumps {
		if p.Permanent {
			kept = append(kept, p)
		}
	}
	g.pumps = kept
	g.endAnimatesAtCleanup()
	g.endSkipsAtCleanup()
	g.endEffectsAtCleanup()
}

// skipPhase is one SkipPhase effect: Player skips the next phase or step in
// Phases -- or, with a Duration$, each one until that duration ends.
// Java builds a command-zone effect holding a BeginPhase replacement that
// exiles itself after one use unless it has a Duration$ (Skip$ True).
type skipPhase struct {
	Player       PlayerID
	Phases       phaseSet
	Each         bool
	UntilCleanup bool
}

// consumeSkip reports whether a skip applies to phase p of the active
// player's turn, using up a one-shot skip.
func (g *Game) consumeSkip(p PhaseType) bool {
	for i, s := range g.skips {
		if s.Player != g.activePlayer || !s.Phases.has(p) {
			continue
		}
		if !s.Each {
			g.skips = append(g.skips[:i:i], g.skips[i+1:]...)
		}
		return true
	}
	return false
}

// endSkipsAtCleanup ends every Duration$ skip (addUntilCommand's
// end-of-turn default).
func (g *Game) endSkipsAtCleanup() {
	kept := g.skips[:0]
	for _, s := range g.skips {
		if !s.UntilCleanup {
			kept = append(kept, s)
		}
	}
	g.skips = kept
}

// isFirstCombat is PhaseHandler.isFirstCombat (nCombatsThisTurn == 1), with
// zero also counted as first: a fixture set straight into a combat step
// through SetTurnState never passed a CombatBegin, and that combat is still
// the turn's first.
func (g *Game) isFirstCombat() bool { return g.combatsThisTurn <= 1 }
