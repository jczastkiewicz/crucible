// The stack: CR 405. One resolvable [Ability] at a time, last on, first off.
//
// Ported from forge-game/src/main/java/forge/game/zone/MagicStack.java
// (1,025 LOC), cut down to the container and CR 405.5/608's resolve loop.
// addSimultaneousStackEntry itself (ordering several triggers that became
// true at once) is ported too, as pushTriggeredAbilities (trigger.go) --
// every real trigger-check function collects its own matches and calls it
// once, rather than calling PushAbility inline as each match is found.
// freezeStack/unfreezeStack (holding new pushes while one ability is
// already resolving) and undoStack only matter once something can push a
// second ability while the first is still open, and casting still has no
// cost-payment or targeting to drive that (control.go's four
// PlayerController methods are the same shape of gap) -- "mechanism now,
// content later," the shape effect.go's Registry already landed in.
package engine

// PushAbility puts an ability on the stack (CR 405.1, 601.2i, 603.3b) and
// emits AbilityActivated. It does not itself decide who is entitled to push
// or in what order -- that is pushTriggeredAbilities' own job (trigger.go)
// for CR 603.3b's APNAP ordering, the same way Move does not itself decide
// whether a zone change is legal. A caller pushing one ability at a time,
// unconditionally (casting a spell, an activated ability once one exists)
// has no ordering question to answer and calls this directly.
func (g *Game) PushAbility(a Ability) {
	g.stack = append(g.stack, a)
	g.sink.Emit(Event{Kind: AbilityActivated, Phase: g.activePhase, Active: g.activePlayer, Actor: a.Controller, Turn: uint16(g.turn), Source: a.Source})
}

// StackLen is how many abilities are waiting to resolve.
func (g *Game) StackLen() int { return len(g.stack) }

// StackTop is the ability that would resolve next, and whether the stack
// holds one -- CR 405's "top of the stack" is the most recently pushed item.
func (g *Game) StackTop() (Ability, bool) {
	if len(g.stack) == 0 {
		return Ability{}, false
	}
	return g.stack[len(g.stack)-1], true
}

// ResolveStack resolves the stack to empty against reg: pop the top ability
// (CR 608.2m -- it leaves the stack before its effect happens, so a
// resolving ability never sees itself still there), dispatch it, emit
// AbilityResolved, then check state-based actions (CR 704.3) before
// resolving what is now on top -- the same pairing beginPhase runs after a
// turn-based action.
//
// This plays out CR 117's priority algorithm for the one case this port can
// reach today: no PlayerController method lets a player respond to anything
// on the stack, so every priority pass is a pass in succession and the top
// item always resolves next, with nothing new arriving on top of it in the
// meantime. Interactive priority -- responding to what is already there,
// MagicStack's freeze/unfreeze around a resolution that pushes another
// ability -- waits on a real activate/cast hook to have anything to prove it
// against (M6).
//
// A resolution's own error stops the loop immediately and reaches the
// caller unchanged (GO-7): a bad card fails its game, not the batch, and
// does not get to resolve whatever was left under it as if nothing
// happened.
func (g *Game) ResolveStack(reg *Registry, controller PlayerController) error {
	for len(g.stack) > 0 && !g.over {
		n := len(g.stack) - 1
		a := g.stack[n]
		g.stack[n] = Ability{}
		g.stack = g.stack[:n]

		if err := reg.Resolve(g, &a); err != nil {
			return err
		}
		g.sink.Emit(Event{Kind: AbilityResolved, Phase: g.activePhase, Active: g.activePlayer, Actor: a.Controller, Turn: uint16(g.turn), Source: a.Source})
		CheckStateBasedActions(g, controller)
	}
	return nil
}
