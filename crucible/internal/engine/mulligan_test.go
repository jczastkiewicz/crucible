package engine_test

import (
	"fmt"
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
)

// scriptedMulliganController answers MulliganKeepHand from a fixed, per-player
// sequence and TuckCardsViaMulligan by always tucking the first N cards of
// whatever hand it is actually handed. Unlike ScriptedController's global
// queues, this does not require predicting which CardIDs a shuffle produces --
// exactly the reason the mulligan tests use a purpose-built fake rather than
// the engine's own ScriptedController.
type scriptedMulliganController struct {
	keep map[engine.PlayerID][]bool
	next map[engine.PlayerID]int
	// onDecide, if set, runs after computing an answer and before returning
	// it -- the hook TestPerformMulligansStopsWhenTheGameIsAlreadyOver uses
	// to make the game end as a side effect of one player's decision, the
	// only way to exercise the mid-loop Over() check without a real SBA
	// interaction inside the mulligan procedure itself.
	onDecide func()
}

func newScriptedMulliganController() *scriptedMulliganController {
	return &scriptedMulliganController{
		keep: map[engine.PlayerID][]bool{},
		next: map[engine.PlayerID]int{},
	}
}

func (c *scriptedMulliganController) queue(p engine.PlayerID, answers ...bool) {
	c.keep[p] = append(c.keep[p], answers...)
}

func (c *scriptedMulliganController) ChooseStartingPlayer(*engine.Game, engine.PlayerID, bool) engine.PlayerID {
	panic("not used by mulligan tests")
}

func (c *scriptedMulliganController) ChooseStartingHand(*engine.Game, engine.PlayerID, [][]engine.CardID) int {
	panic("not used by mulligan tests")
}

func (c *scriptedMulliganController) MulliganKeepHand(_ *engine.Game, decider, _ engine.PlayerID, _ int) bool {
	i := c.next[decider]
	if i >= len(c.keep[decider]) {
		panic(fmt.Sprintf("scriptedMulliganController: ran out of answers for player %d", decider))
	}
	c.next[decider] = i + 1
	answer := c.keep[decider][i]
	if c.onDecide != nil {
		c.onDecide()
	}
	return answer
}

func (c *scriptedMulliganController) TuckCardsViaMulligan(_ *engine.Game, _ engine.PlayerID, hand []engine.CardID, cardsToReturn int) []engine.CardID {
	return hand[:cardsToReturn]
}

// ChooseLegendaryToKeep, DeclareCombatAttackers, DeclareCombatBlockers and
// AssignCombatDamage are never exercised by this controller's own tests --
// no scenario here creates a legend-rule conflict or reaches combat -- but
// the interface still has to be satisfied.
func (c *scriptedMulliganController) ChooseLegendaryToKeep(_ *engine.Game, _ engine.PlayerID, duplicates []engine.CardID) engine.CardID {
	panic("scriptedMulliganController: ChooseLegendaryToKeep was not expected to be called")
}

func (c *scriptedMulliganController) DeclareCombatAttackers(_ *engine.Game, _ engine.PlayerID, eligible []engine.CardID) []engine.CardID {
	panic("scriptedMulliganController: DeclareCombatAttackers was not expected to be called")
}

func (c *scriptedMulliganController) DeclareCombatBlockers(_ *engine.Game, _ engine.PlayerID, attackers []engine.CardID, eligible []engine.CardID) []engine.Block {
	panic("scriptedMulliganController: DeclareCombatBlockers was not expected to be called")
}

func (c *scriptedMulliganController) AssignCombatDamage(_ *engine.Game, _ engine.PlayerID, attacker engine.CardID, blockers []engine.CardID) []engine.DamageAssignment {
	panic("scriptedMulliganController: AssignCombatDamage was not expected to be called")
}

var _ engine.PlayerController = (*scriptedMulliganController)(nil)

func TestPerformMulligansEveryoneKeepsImmediately(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	a, b := g.Players()[0], g.Players()[1]
	var handA, handB []engine.CardID
	for i := 0; i < 7; i++ {
		handA = append(handA, g.NewCard(nil, a, engine.Hand))
		handB = append(handB, g.NewCard(nil, b, engine.Hand))
	}

	c := newScriptedMulliganController()
	c.queue(a, true)
	c.queue(b, true)
	engine.PerformMulligans(g, c, a)

	if got := g.Zone(engine.Hand, a).Cards(); !equalCardIDs(got, handA) {
		t.Errorf("a's hand %v, want unchanged %v", got, handA)
	}
	if got := g.Zone(engine.Hand, b).Cards(); !equalCardIDs(got, handB) {
		t.Errorf("b's hand %v, want unchanged %v", got, handB)
	}
}

// Heads-up London gives no free mulligan: the very first one costs a card.
func TestPerformMulligansOneMulliganTwoPlayer(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	a, b := g.Players()[0], g.Players()[1]
	for i := 0; i < 7; i++ {
		g.NewCard(nil, a, engine.Hand)
	}
	for i := 0; i < 7; i++ {
		g.NewCard(nil, b, engine.Hand)
	}
	poolA := g.Zone(engine.Hand, a).Len() + g.Zone(engine.Library, a).Len()

	c := newScriptedMulliganController()
	c.queue(a, false, true)
	c.queue(b, true)
	engine.PerformMulligans(g, c, a)

	if got := g.Zone(engine.Hand, a).Len(); got != 6 {
		t.Errorf("a's hand has %d cards, want 6 (redrew 7, tucked 1)", got)
	}
	if got := g.Zone(engine.Hand, b).Len(); got != 7 {
		t.Errorf("b's hand has %d cards, want 7 (kept, untouched)", got)
	}
	if got := g.Zone(engine.Hand, a).Len() + g.Zone(engine.Library, a).Len(); got != poolA {
		t.Errorf("a's hand+library pool is %d, want %d -- a mulligan must not create or destroy cards", got, poolA)
	}
}

// CR 103.4: a game with more than two players gives everyone one free
// mulligan -- the first one costs nothing.
func TestPerformMulligansFirstIsFreeWithThreePlayers(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b", "c")
	a, b, cc := g.Players()[0], g.Players()[1], g.Players()[2]
	for _, p := range []engine.PlayerID{a, b, cc} {
		for i := 0; i < 7; i++ {
			g.NewCard(nil, p, engine.Hand)
		}
	}

	ctl := newScriptedMulliganController()
	ctl.queue(a, false, true)
	ctl.queue(b, true)
	ctl.queue(cc, true)
	engine.PerformMulligans(g, ctl, a)

	if got := g.Zone(engine.Hand, a).Len(); got != 7 {
		t.Errorf("a's hand has %d cards, want 7 -- the first mulligan in a 3-player game is free", got)
	}
}

// A hand can shrink all the way to zero, and a player can still choose to
// keep it -- London does not stop a player from mulliganing away everything.
func TestPerformMulligansCanReachAnEmptyHand(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	a, b := g.Players()[0], g.Players()[1]
	for i := 0; i < 7; i++ {
		g.NewCard(nil, a, engine.Hand)
	}
	for i := 0; i < 7; i++ {
		g.NewCard(nil, b, engine.Hand)
	}

	c := newScriptedMulliganController()
	// 7 mulligans at 1, 2, 3, 4, 5, 6, 7 cards tucked leaves 6, 5, 4, 3, 2, 1, 0.
	c.queue(a, false, false, false, false, false, false, false, true)
	c.queue(b, true)
	engine.PerformMulligans(g, c, a)

	if got := g.Zone(engine.Hand, a).Len(); got != 0 {
		t.Errorf("a's hand has %d cards, want 0", got)
	}
}

// canMulligan's cutoff stops offering once taking another mulligan would
// cost more than a hand can pay back -- the last offer is answered, but the
// one after it never asks at all.
func TestPerformMulligansStopsOfferingPastTheCutoff(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	a, b := g.Players()[0], g.Players()[1]
	for i := 0; i < 7; i++ {
		g.NewCard(nil, a, engine.Hand)
	}
	for i := 0; i < 7; i++ {
		g.NewCard(nil, b, engine.Hand)
	}

	c := newScriptedMulliganController()
	// 8 mulligans in a row: canMulligan is still true for the 8th ask (tuck
	// computed from 7 prior mulligans is 7, right at the boundary), but the
	// 9th is never asked -- tuck computed from 8 prior mulligans is 8, past
	// it. Only 8 answers are queued; if a 9th ask happened, the controller
	// would panic on an empty queue and fail the test that way.
	c.queue(a, false, false, false, false, false, false, false, false)
	c.queue(b, true)
	engine.PerformMulligans(g, c, a)

	if got := g.Zone(engine.Hand, a).Len(); got != 0 {
		t.Errorf("a's hand has %d cards, want 0 (8th mulligan tucks every card of a fresh 7, clamped)", got)
	}
}

// If the game ends as a side effect of one player's decision -- nothing
// currently causes that inside the mulligan procedure itself, but a future
// caller answering a decision could concede, and Java's own loop checks for
// exactly this after every ask -- the procedure stops immediately rather
// than asking the next player to decide something in a game that is no
// longer being played.
func TestPerformMulligansStopsWhenTheGameEndsMidLoop(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	a, b := g.Players()[0], g.Players()[1]
	g.Player(a).Life, g.Player(b).Life = 20, 20
	for i := 0; i < 7; i++ {
		g.NewCard(nil, a, engine.Hand)
	}

	c := newScriptedMulliganController()
	c.queue(a, true)
	c.onDecide = func() {
		g.Player(a).Life, g.Player(b).Life = 0, 0
		engine.CheckStateBasedActions(g, c)
	}
	// b has no answers queued: if the loop reached b after a's decision
	// ended the game, this would panic.
	engine.PerformMulligans(g, c, a)

	if !g.Over() {
		t.Fatal("setup: the decision hook did not end the game")
	}
}

func equalCardIDs(a, b []engine.CardID) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
