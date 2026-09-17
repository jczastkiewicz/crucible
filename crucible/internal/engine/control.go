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
// which has 110 abstract methods; only the twenty answerable with today's
// engine are here.
//
// The rest need SpellAbility, targeting, replacement effects and the rest of
// cost payment -- types that do not exist until the stack and layer system
// fully land in M5. Each is added when its own caller is, the same as these
// twenty: mulligans and the starting-player choice have callers in
// GameAction and mulligan/, even though neither is ported yet, and
// ChooseLegendaryToKeep's, DeclareCombatAttackers's, ChooseAttackTarget's,
// DeclareCombatBlockers's, AssignCombatDamage's, DiscardToHandSize's,
// ChooseBattleProtector's, ChooseHybridManaColor's,
// ChoosePayMonocoloredHybrid's, ChoosePayColorlessHybrid's,
// ChoosePayPhyrexian's, ChoosePayHybridPhyrexian's, ChoosePayGeneric's,
// ChoosePayX's, ChoosePaySnow's and ChooseEnchantTarget's own callers
// (resolveLegendRule, action.go; Game.DeclareCombatAttackers and
// Game.assignAttackTargets, attack.go; Game.DeclareCombatBlockers, block.go;
// Game.DealCombatDamage, combatdamage.go; Game.cleanupStep, turn.go;
// assignBattleProtector, action.go; Game.PayManaCost, manapay.go, eight
// times over; Game.CastSpell, castspell.go) are fully built, so the decision
// point can be built ahead of them (Plan Section 1.3).
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

	// ChoosePayMonocoloredHybrid decides whether decider pays a monocoloured
	// hybrid symbol ({2/W}, CR 601.2h) with color or with generic mana
	// instead. generic is the shard's own [mana.Shard.CMC] -- 2 for every
	// {2/W}-shaped symbol printed so far, carried as a parameter rather than
	// assumed so a future symbol with a different generic side is not a
	// silent wrong answer. true pays with color; false leaves the payment to
	// [Game.PayManaCost]'s own extra generic instead. Not re-checked --
	// trust the controller's answer, the same as ChooseLegendaryToKeep.
	ChoosePayMonocoloredHybrid(g *Game, decider PlayerID, color mana.Colors, generic int) bool

	// ChoosePayColorlessHybrid decides whether decider pays a colourless
	// hybrid symbol ({C/W}, CR 601.2h) with color or with {C} instead. color
	// is exactly the one colour the symbol offers ([mana.Shard.Colors]).
	// true pays with color; false substitutes [mana.ShardC] for the symbol
	// instead of adding to generic -- a colourless hybrid's other side is a
	// specific mana type, not an amount, unlike ChoosePayMonocoloredHybrid's
	// generic side. Not re-checked -- trust the controller's answer, the
	// same as ChooseLegendaryToKeep.
	ChoosePayColorlessHybrid(g *Game, decider PlayerID, color mana.Colors) bool

	// ChoosePayPhyrexian decides whether decider pays a single-colour
	// Phyrexian mana symbol ({W/P}, CR 118.4/601.2h) with color or with 2
	// life instead. color is the one colour the symbol offers
	// ([mana.Shard.Colors]). true pays with color; false leaves
	// [Game.PayManaCost] to deduct the life once the rest of the payment is
	// confirmed to succeed, the same as [Game.PayManaCost] never spending
	// mana it cannot finish paying. A hybrid Phyrexian symbol ({B/G/P}, two
	// colours plus life) is a three-way choice this method's boolean shape
	// cannot express -- ChoosePayHybridPhyrexian asks that one. Not
	// re-checked -- trust the controller's answer, the same as
	// ChooseLegendaryToKeep.
	ChoosePayPhyrexian(g *Game, decider PlayerID, color mana.Colors) bool

	// ChoosePayHybridPhyrexian decides how decider pays a hybrid Phyrexian
	// mana symbol ({B/G/P}, CR 118.4/601.2h): with either of its two
	// colours, or with 2 life. colors is exactly the two colours the symbol
	// offers ([mana.Shard.Colors]). Returning one of those two colours pays
	// with it; returning the zero [mana.Colors] pays with 2 life instead,
	// deducted by [Game.PayManaCost] once the rest of the payment is
	// confirmed to succeed, the same guarantee ChoosePayPhyrexian's own
	// life side gets. Not re-checked -- trust the controller's answer, the
	// same as ChooseLegendaryToKeep.
	ChoosePayHybridPhyrexian(g *Game, decider PlayerID, colors mana.Colors) mana.Colors

	// ChoosePayGeneric decides which single type of mana decider spends
	// toward one unit of a mana cost's generic amount (CR 106.6: "any type
	// of mana, including colorless mana, can be used to pay a generic mana
	// cost"; CR 601.2h). [Game.PayManaCost] calls this once per unit of
	// generic still owed, after every colour, hybrid and Phyrexian shard is
	// already resolved -- CR 601.2h's own pips-before-generic order. The
	// return value should be one of mana.ShardW/U/B/R/G/C; it is not
	// re-checked here, but [Pool.Pay]'s own bucket check fails the whole
	// payment if decider's pool does not actually hold what was chosen, the
	// same as an unavailable hybrid colour choice already does.
	ChoosePayGeneric(g *Game, decider PlayerID) mana.Shard

	// ChoosePayX decides the value of X for a cost carrying one or more X
	// symbols (CR 601.2b/107.3f: chosen once per cast, then every X symbol in
	// the cost stands for that same value -- a cost with two X symbols owes
	// twice the chosen amount, not one value each). cost is the whole mana
	// cost being paid, exactly as [Game.PayManaCost] received it, so a real
	// controller can see what accompanies the X symbols and how much floating
	// mana is available before answering; [mana.Cost.CountX] is how many X
	// symbols it carries.
	//
	// The return value is folded into the cost's generic amount (chosen
	// value times [mana.Cost.CountX]) before anything else resolves, the
	// same "ahead of every other shard" position CR 601.2b's own ordering
	// puts X's announcement in. A negative answer is not re-checked here,
	// the same as an unavailable hybrid colour choice: [Game.PayManaCost]
	// reports payment failure rather than trusting a value CR 601.2b's own
	// "non-negative integer" rule forbids.
	ChoosePayX(g *Game, decider PlayerID, cost mana.Cost) int

	// ChoosePaySnow decides which color of floating snow mana decider spends
	// on one snow ({S}, CR 106.3a) symbol. [Game.PayManaCost] calls this once
	// per {S} symbol the cost carries, independently -- unlike ChoosePayX,
	// two {S} symbols in the same cost are two separate questions, not one
	// value reused, since each can be paid with a different color's snow
	// mana. The return value should be one of mana.ShardW/U/B/R/G/C; it is
	// not re-checked here, but [Pool.PayWithSnow]'s own bucket check fails
	// the whole payment if decider's pool does not actually hold snow mana of
	// the color chosen, the same as an unavailable hybrid colour choice
	// already does. Snow-tagged mana still pays a same-color pip or a
	// generic unit the same as plain mana of that color -- this method exists
	// only for the one requirement plain mana cannot cover.
	ChoosePaySnow(g *Game, decider PlayerID) mana.Shard

	// ChooseEnchantTarget decides which permanent an Aura being cast attaches
	// to (CR 601.2c, Game.CastSpell, castspell.go). eligible always has at
	// least two elements: CastSpell assigns a lone eligible target
	// automatically without asking, the same "nothing meaningful to decide"
	// reasoning assignAttackTargets already applies. The return value should
	// be one of eligible's elements, and is not re-checked -- trust the
	// controller's answer, the same as ChooseLegendaryToKeep.
	ChooseEnchantTarget(g *Game, decider PlayerID, aura CardID, eligible []CardID) CardID
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
	monoHybrid      []bool
	colorlessHybrid []bool
	phyrexian       []bool
	hybridPhyrexian []mana.Colors
	genericMana     []mana.Shard
	xValues         []int
	snowMana        []mana.Shard
	enchantTargets  []CardID
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

// ChooseStartingPlayer returns the next answer QueueStartingPlayer queued.
func (c *ScriptedController) ChooseStartingPlayer(_ *Game, _ PlayerID, _ bool) PlayerID {
	if len(c.startingPlayers) == 0 {
		panic(scriptExhausted("starting player"))
	}
	v := c.startingPlayers[0]
	c.startingPlayers = c.startingPlayers[1:]
	return v
}

// ChooseStartingHand returns the next answer QueueStartingHand queued.
func (c *ScriptedController) ChooseStartingHand(_ *Game, _ PlayerID, _ [][]CardID) int {
	if len(c.startingHands) == 0 {
		panic(scriptExhausted("starting hand"))
	}
	v := c.startingHands[0]
	c.startingHands = c.startingHands[1:]
	return v
}

// MulliganKeepHand returns the next answer QueueKeepHand queued.
func (c *ScriptedController) MulliganKeepHand(_ *Game, _, _ PlayerID, _ int) bool {
	if len(c.keepHand) == 0 {
		panic(scriptExhausted("keep hand"))
	}
	v := c.keepHand[0]
	c.keepHand = c.keepHand[1:]
	return v
}

// TuckCardsViaMulligan returns the next answer QueueTuck queued.
func (c *ScriptedController) TuckCardsViaMulligan(_ *Game, _ PlayerID, _ []CardID, _ int) []CardID {
	if len(c.tucked) == 0 {
		panic(scriptExhausted("tuck via mulligan"))
	}
	v := c.tucked[0]
	c.tucked = c.tucked[1:]
	return v
}

// ChooseLegendaryToKeep returns the next answer QueueLegendaryToKeep queued.
func (c *ScriptedController) ChooseLegendaryToKeep(_ *Game, _ PlayerID, _ []CardID) CardID {
	if len(c.legendaryKeep) == 0 {
		panic(scriptExhausted("legendary to keep"))
	}
	v := c.legendaryKeep[0]
	c.legendaryKeep = c.legendaryKeep[1:]
	return v
}

// DeclareCombatAttackers returns the next answer QueueAttackers queued.
func (c *ScriptedController) DeclareCombatAttackers(_ *Game, _ PlayerID, _ []CardID) []CardID {
	if len(c.attackers) == 0 {
		panic(scriptExhausted("attackers"))
	}
	v := c.attackers[0]
	c.attackers = c.attackers[1:]
	return v
}

// ChooseAttackTarget returns the next answer QueueAttackTarget queued.
func (c *ScriptedController) ChooseAttackTarget(_ *Game, _ PlayerID, _ CardID, _ []EntityID) EntityID {
	if len(c.attackTargets) == 0 {
		panic(scriptExhausted("attack target"))
	}
	v := c.attackTargets[0]
	c.attackTargets = c.attackTargets[1:]
	return v
}

// DeclareCombatBlockers returns the next answer QueueBlocks queued.
func (c *ScriptedController) DeclareCombatBlockers(_ *Game, _ PlayerID, _ []CardID, _ []CardID) []Block {
	if len(c.blocks) == 0 {
		panic(scriptExhausted("blocks"))
	}
	v := c.blocks[0]
	c.blocks = c.blocks[1:]
	return v
}

// AssignCombatDamage returns the next answer QueueDamageAssignment queued.
func (c *ScriptedController) AssignCombatDamage(_ *Game, _ PlayerID, _ CardID, _ []CardID) []DamageAssignment {
	if len(c.damage) == 0 {
		panic(scriptExhausted("damage assignment"))
	}
	v := c.damage[0]
	c.damage = c.damage[1:]
	return v
}

// DiscardToHandSize returns the next answer QueueDiscard queued.
func (c *ScriptedController) DiscardToHandSize(_ *Game, _ PlayerID, _ []CardID, _ int) []CardID {
	if len(c.discards) == 0 {
		panic(scriptExhausted("discard"))
	}
	v := c.discards[0]
	c.discards = c.discards[1:]
	return v
}

// ChooseBattleProtector returns the next answer QueueBattleProtector queued.
func (c *ScriptedController) ChooseBattleProtector(_ *Game, _ PlayerID, _ CardID, _ []PlayerID) PlayerID {
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

// ChooseHybridManaColor returns the next answer QueueHybridManaColor queued.
func (c *ScriptedController) ChooseHybridManaColor(_ *Game, _ PlayerID, _ mana.Colors) mana.Colors {
	if len(c.hybridMana) == 0 {
		panic(scriptExhausted("hybrid mana color"))
	}
	v := c.hybridMana[0]
	c.hybridMana = c.hybridMana[1:]
	return v
}

// QueuePayMonocoloredHybrid appends the answer to the next
// ChoosePayMonocoloredHybrid call.
func (c *ScriptedController) QueuePayMonocoloredHybrid(payColor bool) {
	c.monoHybrid = append(c.monoHybrid, payColor)
}

// ChoosePayMonocoloredHybrid returns the next answer QueuePayMonocoloredHybrid queued.
func (c *ScriptedController) ChoosePayMonocoloredHybrid(_ *Game, _ PlayerID, _ mana.Colors, _ int) bool {
	if len(c.monoHybrid) == 0 {
		panic(scriptExhausted("pay monocolored hybrid"))
	}
	v := c.monoHybrid[0]
	c.monoHybrid = c.monoHybrid[1:]
	return v
}

// QueuePayColorlessHybrid appends the answer to the next
// ChoosePayColorlessHybrid call.
func (c *ScriptedController) QueuePayColorlessHybrid(payColor bool) {
	c.colorlessHybrid = append(c.colorlessHybrid, payColor)
}

// ChoosePayColorlessHybrid returns the next answer QueuePayColorlessHybrid queued.
func (c *ScriptedController) ChoosePayColorlessHybrid(_ *Game, _ PlayerID, _ mana.Colors) bool {
	if len(c.colorlessHybrid) == 0 {
		panic(scriptExhausted("pay colorless hybrid"))
	}
	v := c.colorlessHybrid[0]
	c.colorlessHybrid = c.colorlessHybrid[1:]
	return v
}

// QueuePayPhyrexian appends the answer to the next ChoosePayPhyrexian call.
func (c *ScriptedController) QueuePayPhyrexian(payColor bool) {
	c.phyrexian = append(c.phyrexian, payColor)
}

// ChoosePayPhyrexian returns the next answer QueuePayPhyrexian queued.
func (c *ScriptedController) ChoosePayPhyrexian(_ *Game, _ PlayerID, _ mana.Colors) bool {
	if len(c.phyrexian) == 0 {
		panic(scriptExhausted("pay phyrexian"))
	}
	v := c.phyrexian[0]
	c.phyrexian = c.phyrexian[1:]
	return v
}

// QueuePayHybridPhyrexian appends the answer to the next
// ChoosePayHybridPhyrexian call. Queue the zero [mana.Colors] for "pay with
// life instead."
func (c *ScriptedController) QueuePayHybridPhyrexian(colors mana.Colors) {
	c.hybridPhyrexian = append(c.hybridPhyrexian, colors)
}

// ChoosePayHybridPhyrexian returns the next answer QueuePayHybridPhyrexian queued.
func (c *ScriptedController) ChoosePayHybridPhyrexian(_ *Game, _ PlayerID, _ mana.Colors) mana.Colors {
	if len(c.hybridPhyrexian) == 0 {
		panic(scriptExhausted("pay hybrid phyrexian"))
	}
	v := c.hybridPhyrexian[0]
	c.hybridPhyrexian = c.hybridPhyrexian[1:]
	return v
}

// QueuePayGeneric appends the answer to the next ChoosePayGeneric call.
func (c *ScriptedController) QueuePayGeneric(s mana.Shard) {
	c.genericMana = append(c.genericMana, s)
}

// ChoosePayGeneric returns the next answer QueuePayGeneric queued.
func (c *ScriptedController) ChoosePayGeneric(_ *Game, _ PlayerID) mana.Shard {
	if len(c.genericMana) == 0 {
		panic(scriptExhausted("pay generic"))
	}
	v := c.genericMana[0]
	c.genericMana = c.genericMana[1:]
	return v
}

// QueuePayX appends the answer to the next ChoosePayX call.
func (c *ScriptedController) QueuePayX(x int) {
	c.xValues = append(c.xValues, x)
}

// ChoosePayX returns the next answer QueuePayX queued.
func (c *ScriptedController) ChoosePayX(_ *Game, _ PlayerID, _ mana.Cost) int {
	if len(c.xValues) == 0 {
		panic(scriptExhausted("pay x"))
	}
	v := c.xValues[0]
	c.xValues = c.xValues[1:]
	return v
}

// QueuePaySnow appends the answer to the next ChoosePaySnow call.
func (c *ScriptedController) QueuePaySnow(s mana.Shard) {
	c.snowMana = append(c.snowMana, s)
}

// ChoosePaySnow returns the next answer QueuePaySnow queued.
func (c *ScriptedController) ChoosePaySnow(_ *Game, _ PlayerID) mana.Shard {
	if len(c.snowMana) == 0 {
		panic(scriptExhausted("pay snow"))
	}
	v := c.snowMana[0]
	c.snowMana = c.snowMana[1:]
	return v
}

// QueueEnchantTarget appends the answer to the next ChooseEnchantTarget call.
func (c *ScriptedController) QueueEnchantTarget(host CardID) {
	c.enchantTargets = append(c.enchantTargets, host)
}

// ChooseEnchantTarget returns the next answer QueueEnchantTarget queued.
func (c *ScriptedController) ChooseEnchantTarget(_ *Game, _ PlayerID, _ CardID, _ []CardID) CardID {
	if len(c.enchantTargets) == 0 {
		panic(scriptExhausted("enchant target"))
	}
	v := c.enchantTargets[0]
	c.enchantTargets = c.enchantTargets[1:]
	return v
}

// scriptExhausted is what a queue running dry mid-scenario panics with. It
// names the decision kind, because a fixture with several queues needs to
// know which one came up short.
func scriptExhausted(kind string) string {
	return fmt.Sprintf("engine: scripted controller ran out of %s decisions", kind)
}
