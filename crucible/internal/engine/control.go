// The controller interface: where the game asks a player to decide
// something, and the fixture-driven implementation that answers from a
// script instead of a person or an AI.

package engine

import (
	"fmt"

	"github.com/jczastkiewicz/crucible/internal/mana"
)

// PlayerController is where the game asks a player to decide something.
// Ported from forge-game/src/main/java/forge/game/player/PlayerController.java,
// which has 110 abstract methods; only the twelve answerable with today's
// engine are here.
//
// The rest need SpellAbility, targeting, replacement effects and the rest of
// cost payment -- types that do not exist until the stack and layer system
// fully land in M5. Each is added when its own caller is, the same as these
// twelve: mulligans and the starting-player choice have callers in
// GameAction and mulligan/, even though neither is ported yet, and
// ChooseLegendaryToKeep's, DeclareCombatAttackers's, ChooseAttackTarget's,
// DeclareCombatBlockers's, AssignCombatDamage's, DiscardToHandSize's,
// ChooseBattleProtector's and ChooseHybridManaColor's own callers
// (resolveLegendRule, action.go; Game.DeclareCombatAttackers and
// Game.assignAttackTargets, attack.go; Game.DeclareCombatBlockers, block.go;
// Game.DealCombatDamage, combatdamage.go; Game.cleanupStep, turn.go;
// assignBattleProtector, action.go; Game.PayManaCost, manapay.go) are fully
// built, so the decision point can be built ahead of them (Plan Section 1.3).
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

	// ChooseLegendaryToKeep decides which of several legendary permanents
	// sharing a name decider keeps when the legend rule applies; the rest
	// go to their owner's graveyard (resolveLegendRule, action.go).
	// duplicates always has at least two elements, and the return value
	// must be one of them.
	ChooseLegendaryToKeep(g *Game, decider PlayerID, duplicates []CardID) CardID

	// DeclareCombatAttackers decides which of decider's eligible creatures attack
	// (CR 508.1, Game.DeclareCombatAttackers, attack.go). The return value is a
	// subset of eligible, which is never empty (Game.DeclareCombatAttackers does
	// not call this otherwise) -- an empty return is a legal answer, the
	// active player declining to attack with anything.
	DeclareCombatAttackers(g *Game, decider PlayerID, eligible []CardID) []CardID

	// ChooseAttackTarget decides what a single declared attacker is
	// attacking -- the defending player, or one of the planeswalkers/battles
	// they control (CR 508.1d, Game.assignAttackTargets, attack.go). eligible
	// always has at least two elements: Game.assignAttackTargets assigns a
	// lone eligible target automatically without asking. The return value
	// should be one of eligible's elements, and is not re-checked -- trust
	// the controller's answer, the same as ChooseLegendaryToKeep.
	ChooseAttackTarget(g *Game, decider PlayerID, attacker CardID, eligible []EntityID) EntityID

	// DeclareCombatBlockers decides which of decider's eligible creatures block
	// which attacker (CR 509.1, Game.DeclareCombatBlockers, block.go). eligible is
	// never empty. The return value need not use every element of eligible
	// or attackers -- declining to block anything is legal -- and is not
	// re-checked for legality beyond what Game.DeclareCombatBlockers already
	// filtered (block.go's doc comment has the reasons why).
	DeclareCombatBlockers(g *Game, decider PlayerID, attackers []CardID, eligible []CardID) []Block

	// AssignCombatDamage decides how a gang-blocked attacker's combat damage
	// divides among the creatures blocking it (CR 510.1c,
	// Game.DealCombatDamage, combatdamage.go). Only called when len(blockers)
	// > 1 -- a single blocker gets the attacker's full power automatically,
	// nothing to decide. blockers always has at least two elements; the
	// return value's order is the order the attacking player assigns in, and
	// is not re-checked for the "lethal before moving on" requirement CR
	// 510.1c imposes -- trust the controller's answer, the same as
	// ChooseLegendaryToKeep.
	AssignCombatDamage(g *Game, decider PlayerID, attacker CardID, blockers []CardID) []DamageAssignment

	// DiscardToHandSize decides which of decider's hand to discard at
	// cleanup (CR 514.1, Game.cleanupStep, turn.go). Only called when hand
	// has more than MaxHandSize cards; count is exactly how many the
	// returned slice must have (hand.Len() - MaxHandSize), a constraint not
	// re-checked here -- trust the controller's answer, the same as
	// ChooseLegendaryToKeep.
	DiscardToHandSize(g *Game, decider PlayerID, hand []CardID, count int) []CardID

	// ChooseBattleProtector decides which opponent defends decider's Battle
	// (CR 704.5w/704.5x, assignBattleProtector, action.go). eligible is
	// never empty. The return value should be one of eligible's elements,
	// and is not re-checked -- trust the controller's answer, the same as
	// ChooseLegendaryToKeep.
	ChooseBattleProtector(g *Game, decider PlayerID, battle CardID, eligible []PlayerID) PlayerID

	// ChooseHybridManaColor decides which of a two-colour hybrid mana
	// symbol's colours decider pays with (CR 601.2h, [Game.PayManaCost],
	// manapay.go). options is exactly the two colours the symbol offers
	// ([mana.Shard.Colors]); the return value should be exactly one of
	// them, and is not re-checked -- trust the controller's answer, the
	// same as ChooseLegendaryToKeep.
	ChooseHybridManaColor(g *Game, decider PlayerID, options mana.Colors) mana.Colors
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
	legendaryKeep   []CardID
	attackers       [][]CardID
	attackTargets   []EntityID
	blocks          [][]Block
	damage          [][]DamageAssignment
	discards        [][]CardID
	battleProtector []PlayerID
	hybridMana      []mana.Colors
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

// QueueLegendaryToKeep appends the answer to the next ChooseLegendaryToKeep
// call.
func (c *ScriptedController) QueueLegendaryToKeep(id CardID) {
	c.legendaryKeep = append(c.legendaryKeep, id)
}

// QueueAttackers appends the answer to the next DeclareCombatAttackers call. An
// empty or nil cards declines to attack with anything, a legal answer that
// still consumes the queue slot.
func (c *ScriptedController) QueueAttackers(cards []CardID) {
	c.attackers = append(c.attackers, cards)
}

// QueueAttackTarget appends the answer to the next ChooseAttackTarget call.
func (c *ScriptedController) QueueAttackTarget(target EntityID) {
	c.attackTargets = append(c.attackTargets, target)
}

// QueueBlocks appends the answer to the next DeclareCombatBlockers call. Nil
// declines to block anything, a legal answer that still consumes the queue
// slot.
func (c *ScriptedController) QueueBlocks(blocks []Block) {
	c.blocks = append(c.blocks, blocks)
}

// QueueDamageAssignment appends the answer to the next AssignCombatDamage
// call.
func (c *ScriptedController) QueueDamageAssignment(assignment []DamageAssignment) {
	c.damage = append(c.damage, assignment)
}

// QueueDiscard appends the answer to the next DiscardToHandSize call.
func (c *ScriptedController) QueueDiscard(cards []CardID) {
	c.discards = append(c.discards, cards)
}

// QueueBattleProtector appends the answer to the next ChooseBattleProtector
// call.
func (c *ScriptedController) QueueBattleProtector(p PlayerID) {
	c.battleProtector = append(c.battleProtector, p)
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

func (c *ScriptedController) ChooseLegendaryToKeep(g *Game, decider PlayerID, duplicates []CardID) CardID {
	if len(c.legendaryKeep) == 0 {
		panic(scriptExhausted("legendary to keep"))
	}
	v := c.legendaryKeep[0]
	c.legendaryKeep = c.legendaryKeep[1:]
	return v
}

func (c *ScriptedController) DeclareCombatAttackers(g *Game, decider PlayerID, eligible []CardID) []CardID {
	if len(c.attackers) == 0 {
		panic(scriptExhausted("attackers"))
	}
	v := c.attackers[0]
	c.attackers = c.attackers[1:]
	return v
}

func (c *ScriptedController) ChooseAttackTarget(g *Game, decider PlayerID, attacker CardID, eligible []EntityID) EntityID {
	if len(c.attackTargets) == 0 {
		panic(scriptExhausted("attack target"))
	}
	v := c.attackTargets[0]
	c.attackTargets = c.attackTargets[1:]
	return v
}

func (c *ScriptedController) DeclareCombatBlockers(g *Game, decider PlayerID, attackers []CardID, eligible []CardID) []Block {
	if len(c.blocks) == 0 {
		panic(scriptExhausted("blocks"))
	}
	v := c.blocks[0]
	c.blocks = c.blocks[1:]
	return v
}

func (c *ScriptedController) AssignCombatDamage(g *Game, decider PlayerID, attacker CardID, blockers []CardID) []DamageAssignment {
	if len(c.damage) == 0 {
		panic(scriptExhausted("damage assignment"))
	}
	v := c.damage[0]
	c.damage = c.damage[1:]
	return v
}

func (c *ScriptedController) DiscardToHandSize(g *Game, decider PlayerID, hand []CardID, count int) []CardID {
	if len(c.discards) == 0 {
		panic(scriptExhausted("discard"))
	}
	v := c.discards[0]
	c.discards = c.discards[1:]
	return v
}

func (c *ScriptedController) ChooseBattleProtector(g *Game, decider PlayerID, battle CardID, eligible []PlayerID) PlayerID {
	if len(c.battleProtector) == 0 {
		panic(scriptExhausted("battle protector"))
	}
	v := c.battleProtector[0]
	c.battleProtector = c.battleProtector[1:]
	return v
}

// QueueHybridManaColor appends the answer to the next ChooseHybridManaColor
// call.
func (c *ScriptedController) QueueHybridManaColor(color mana.Colors) {
	c.hybridMana = append(c.hybridMana, color)
}

func (c *ScriptedController) ChooseHybridManaColor(g *Game, decider PlayerID, options mana.Colors) mana.Colors {
	if len(c.hybridMana) == 0 {
		panic(scriptExhausted("hybrid mana color"))
	}
	v := c.hybridMana[0]
	c.hybridMana = c.hybridMana[1:]
	return v
}

// scriptExhausted is what a queue running dry mid-scenario panics with. It
// names the decision kind, because a fixture with several queues needs to
// know which one came up short.
func scriptExhausted(kind string) string {
	return fmt.Sprintf("engine: scripted controller ran out of %s decisions", kind)
}
