// The controller interface: where the game asks a player to decide
// something, and the fixture-driven implementation that answers from a
// script instead of a person or an AI.

package engine

import "fmt"

// PlayerController is where the game asks a player to decide something.
// Ported from forge-game/src/main/java/forge/game/player/PlayerController.java,
// which has 110 abstract methods; only the four answerable with today's
// engine are here.
//
// The rest need SpellAbility, Combat, targeting, replacement effects and cost
// payment -- types that do not exist until the stack, combat and layer system
// land in M5. Each is added when its own caller is, the same as these four:
// mulligans and the starting-player choice have callers in GameAction and
// mulligan/, even though neither is ported yet, so the decision point can be
// built ahead of them (Plan Section 1.3).
//
// Forge instantiates one controller per player. Go's methods take the
// deciding player as an explicit PlayerID instead of binding an instance to
// one seat, so a single ScriptedController answers for every player in a
// fixture without a controller-per-player wiring step (PORT-1) -- and so an
// implementation carries no per-player state, the same reasoning GO-2 applies
// to the rest of the engine.
type PlayerController interface {
	// ChooseStartingPlayer decides who takes the first turn. decider is the
	// player being asked -- the winner of a coin flip on game one, the loser
	// of the previous game after that -- and the return value need not be
	// decider: choosing to put an opponent on the play is choosing draw.
	ChooseStartingPlayer(g *Game, decider PlayerID, isFirstGame bool) PlayerID

	// ChooseStartingHand picks among several candidate opening hands, for
	// variants that deal more than one (Backup, extra-hand modes). hands[i]
	// is the i-th candidate's cards, and the return value is its index.
	ChooseStartingHand(g *Game, decider PlayerID, hands [][]CardID) int

	// MulliganKeepHand asks whether decider keeps their current hand.
	// firstPlayer is who is on the play this game, which the free-mulligan
	// count keys off; cardsToReturn is not what keeping costs -- keeping is
	// always free under London, the only rule this port has -- it is what
	// taking one *more* mulligan would cost, so the decision is informed by
	// the price of saying no.
	MulliganKeepHand(g *Game, decider PlayerID, firstPlayer PlayerID, cardsToReturn int) bool

	// TuckCardsViaMulligan picks which cards from hand go to the bottom of
	// the library under a London or Houston mulligan. The return value is a
	// subset of hand of length cardsToReturn. Moving the cards is the
	// mulligan rule's job, not the controller's -- Forge's own callers
	// (LondonMulligan, HoustonMulligan) do the move themselves after asking.
	TuckCardsViaMulligan(g *Game, decider PlayerID, hand []CardID, cardsToReturn int) []CardID
}

// ScriptedController answers every decision from a pre-loaded queue, one per
// method. It is what a TEST-5 fixture runs against: no AI, no heuristics, so
// a scenario's outcome is deterministic and any divergence from the Java
// oracle is a rules bug, never an AI one (Plan Section 3.3, Layer 2).
//
// A queue running dry mid-game is a fixture-authoring mistake, not a rules
// question a card script could cause, so it panics rather than returning a
// zero value that would silently pass the scenario for the wrong reason
// (GO-7).
type ScriptedController struct {
	startingPlayers []PlayerID
	startingHands   []int
	keepHand        []bool
	tucked          [][]CardID
}

// NewScriptedController builds a controller with no decisions queued yet.
func NewScriptedController() *ScriptedController {
	return &ScriptedController{}
}

// QueueStartingPlayer appends the answer to the next ChooseStartingPlayer call.
func (c *ScriptedController) QueueStartingPlayer(p PlayerID) {
	c.startingPlayers = append(c.startingPlayers, p)
}

// QueueStartingHand appends the answer to the next ChooseStartingHand call.
func (c *ScriptedController) QueueStartingHand(index int) {
	c.startingHands = append(c.startingHands, index)
}

// QueueKeepHand appends the answer to the next MulliganKeepHand call.
func (c *ScriptedController) QueueKeepHand(keep bool) {
	c.keepHand = append(c.keepHand, keep)
}

// QueueTuck appends the answer to the next TuckCardsViaMulligan call.
func (c *ScriptedController) QueueTuck(cards []CardID) {
	c.tucked = append(c.tucked, cards)
}

func (c *ScriptedController) ChooseStartingPlayer(g *Game, decider PlayerID, isFirstGame bool) PlayerID {
	if len(c.startingPlayers) == 0 {
		panic(scriptExhausted("starting player"))
	}
	v := c.startingPlayers[0]
	c.startingPlayers = c.startingPlayers[1:]
	return v
}

func (c *ScriptedController) ChooseStartingHand(g *Game, decider PlayerID, hands [][]CardID) int {
	if len(c.startingHands) == 0 {
		panic(scriptExhausted("starting hand"))
	}
	v := c.startingHands[0]
	c.startingHands = c.startingHands[1:]
	return v
}

func (c *ScriptedController) MulliganKeepHand(g *Game, decider, firstPlayer PlayerID, cardsToReturn int) bool {
	if len(c.keepHand) == 0 {
		panic(scriptExhausted("keep hand"))
	}
	v := c.keepHand[0]
	c.keepHand = c.keepHand[1:]
	return v
}

func (c *ScriptedController) TuckCardsViaMulligan(g *Game, decider PlayerID, hand []CardID, cardsToReturn int) []CardID {
	if len(c.tucked) == 0 {
		panic(scriptExhausted("tuck via mulligan"))
	}
	v := c.tucked[0]
	c.tucked = c.tucked[1:]
	return v
}

// scriptExhausted is what a queue running dry mid-scenario panics with. It
// names the decision kind, because a fixture with several queues needs to
// know which one came up short.
func scriptExhausted(kind string) string {
	return fmt.Sprintf("engine: scripted controller ran out of %s decisions", kind)
}
