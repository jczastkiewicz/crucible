// Combat damage: CR 510.1-510.4 (the first strike sub-step included), CR
// 702.19 (trample).

package engine

import "github.com/jczastkiewicz/crucible/internal/cardtype"

// DealFirstStrikeDamage is CR 510.4's first sub-step: only creatures with
// first strike or double strike deal damage. A safe no-op when nothing in
// combat has either keyword -- a fixture need not check first before
// calling it, the same "the game decides what is legal" reasoning
// `Game.DeclareCombatAttackers` already applies to an empty eligible list.
//
// This and `DealCombatDamage` share `dealCombatDamageStep`; see its own doc
// comment for the shape both steps follow.
func (g *Game) DealFirstStrikeDamage(controller PlayerController) {
	g.dealCombatDamageStep(controller, true)
}

// DealCombatDamage is CR 510.4's regular step: every creature without first
// strike deals damage, and every creature with double strike deals damage
// again. When nothing in combat has first strike or double strike, this is
// also the entirety of CR 510.1-510.3 -- the ordinary case, and the only
// step most fixtures need to call.
//
// See `dealCombatDamageStep`'s doc comment for what each exchange computes,
// including trample (CR 702.19).
func (g *Game) DealCombatDamage(controller PlayerController) {
	g.dealCombatDamageStep(controller, false)
}

// dealCombatDamageStep is one damage sub-step, first-strike or regular
// (`firstStrike` selects which). A creature deals damage in a step per
// `dealsInStep`'s doc comment.
//
// All of a step's damage is simultaneous (CR 510.2), but no lifelink exists
// yet and nothing cares about the order two life totals change in, so
// applying one attacker's exchange at a time produces the same result as
// computing every amount first and applying them together --
// checkDamageDoneTriggersToCard/ToPlayer (trigger.go), dealt with below,
// fire per exchange rather than once for the whole step for the identical
// reason: nothing here can tell the difference yet.
//
// An unblocked attacker deals its power to whatever it's attacking (CR
// 508.1d) -- a player, planeswalker or battle, via dealAttackTargetDamage.
// A blocked attacker with exactly one live blocker and no trample exchanges full power for
// full power automatically, no decision needed, the same "nothing
// meaningful to decide" reasoning DeclareCombatAttackers/Blockers use for
// an empty eligible list. A gang-blocked attacker (more than one live
// blocker) asks the attacking player how to divide its power among them
// (AssignCombatDamage, CR 510.1c) -- trusted the same way
// ChooseLegendaryToKeep's answer is, including the "lethal before moving
// on" ordering constraint CR 510.1c itself imposes.
//
// Trample (CR 702.19) changes only how much of a blocked attacker's power
// reaches its blocker(s) versus what it's attacking, not who decides:
// against a single live blocker, `lethalDamage` computes the minimum this
// port can assign it (the game deciding, since no decision was being asked
// in that case anyway) and the rest tramples over, unless lethal can't be
// computed at all (`lethalDamage`'s own doc comment) -- against a
// gang-blocked attacker, whatever the controller's AssignCombatDamage
// answer leaves unassigned across all named blockers tramples over instead
// of being wasted. A non-trampler's unassigned remainder is wasted exactly
// as before this method existed.
//
// A creature that has left the battlefield since DeclareCombatBlockers --
// reachable now that first strike damage can kill a creature before the
// regular step runs, CR 510.1c's "attacker remains blocked but a removed
// blocker gets nothing" exception -- deals no damage and receives none: an
// attacker checks `g.alive` before doing anything at all, and dead blockers
// are filtered out of its list before either side of the exchange runs.
func (g *Game) dealCombatDamageStep(controller PlayerController, firstStrike bool) {
	blockersOf := make(map[CardID][]CardID, len(g.combat.Blocks))
	for _, b := range g.combat.Blocks {
		blockersOf[b.Attacker] = append(blockersOf[b.Attacker], b.Blocker)
	}

	for _, atkID := range g.combat.Attackers {
		if !g.alive(atkID) {
			continue
		}
		atk := g.Card(atkID)
		declaredBlockers := blockersOf[atkID]
		var blockers []CardID
		for _, id := range declaredBlockers {
			if g.alive(id) {
				blockers = append(blockers, id)
			}
		}

		if dealsInStep(atk, firstStrike) {
			if power, ok := atk.Power(); ok && power > 0 {
				g.dealAttackerDamage(controller, atkID, power, blockers, len(declaredBlockers) == 0)
			}
		}

		for _, blkID := range blockers {
			blk := g.Card(blkID)
			if dealsInStep(blk, firstStrike) {
				if bp, ok := blk.Power(); ok && bp > 0 {
					g.dealPermanentDamage(blkID, atkID, bp, blk.HasKeyword("Deathtouch"))
				}
			}
		}
	}
}

// dealsInStep reports whether c deals damage in the first-strike step
// (firstStrike true) or the regular step (false) -- CR 510.4, 702.4b, 702.7c:
// double strike acts in both, first strike (alone) only in the first, and
// everything else only in the regular one.
func dealsInStep(c *Card, firstStrike bool) bool {
	fs, ds := c.HasKeyword("First Strike"), c.HasKeyword("Double Strike")
	if firstStrike {
		return fs || ds
	}
	return !fs || ds
}

// alive reports whether id is still the card it was declared into combat as
// -- on the battlefield, not replaced by a state-based action that ran
// since (most likely the previous damage sub-step's own kills).
func (g *Game) alive(id CardID) bool {
	return g.Card(id).Zone == Battlefield
}

// dealAttackerDamage is attacker's half of dealCombatDamageStep's exchange:
// where its power goes, given liveBlockers (already filtered to what's
// still on the battlefield) and wasUnblocked (true only when the attacker
// was never blocked at all, as opposed to blocked by creatures that have
// since died -- CR 510.1c treats the two differently). Damage that reaches
// past every blocker goes to whatever attacker is attacking (CR 508.1d) --
// a player, a planeswalker, or a battle -- not always the defending player,
// via dealAttackTargetDamage.
func (g *Game) dealAttackerDamage(controller PlayerController, attacker CardID, power int, liveBlockers []CardID, wasUnblocked bool) {
	atk := g.Card(attacker)
	deathtouch := atk.HasKeyword("Deathtouch")
	trample := atk.HasKeyword("Trample")

	switch len(liveBlockers) {
	case 0:
		// Unblocked always hits its target (CR 510.1a). Blocked with every
		// blocker since dead hits it too, but only with trample (CR
		// 702.19e) -- without it, CR 510.1c leaves the attacker dealing
		// nothing at all, not even to what it's attacking.
		if wasUnblocked || trample {
			g.dealAttackTargetDamage(attacker, power, deathtouch)
		}

	case 1:
		blocker := liveBlockers[0]
		toBlocker := power
		if trample {
			if lethal, ok := lethalDamage(g.Card(blocker), deathtouch); ok && lethal < power {
				toBlocker = lethal
			}
		}
		g.dealPermanentDamage(attacker, blocker, toBlocker, deathtouch)
		if trample && power > toBlocker {
			g.dealAttackTargetDamage(attacker, power-toBlocker, deathtouch)
		}

	default:
		assigned := 0
		for _, a := range controller.AssignCombatDamage(g, atk.Controller, attacker, liveBlockers) {
			g.dealPermanentDamage(attacker, a.Blocker, a.Amount, deathtouch)
			assigned += a.Amount
		}
		if trample && power > assigned {
			g.dealAttackTargetDamage(attacker, power-assigned, deathtouch)
		}
	}
}

// dealAttackTargetDamage sends amount to whatever attacker is attacking (CR
// 508.1d, attack.go) -- a player (dealPlayerDamage) or a planeswalker/battle
// (dealPermanentDamage) -- rather than assuming it's always the defending
// player directly, the way this port's combat did before a target could be
// anything else.
func (g *Game) dealAttackTargetDamage(attacker CardID, amount int, deathtouch bool) {
	target := g.combat.AttackTargets[attacker]
	if pid, ok := target.AsPlayer(); ok {
		g.dealPlayerDamage(attacker, pid, amount)
		return
	}
	cid, _ := target.AsCard()
	g.dealPermanentDamage(attacker, cid, amount, deathtouch)
}

// lethalDamage is CR 510.1c/702.19c's "lethal damage": 1 from a deathtouch
// source, otherwise however much toughness the target has left this turn.
// The second return value is false when that can't be computed at all (an
// unresolvable toughness -- `Toughness`'s own `*`/Count$ gap, game-state.md);
// its caller treats that the same as "not trampling this blocker" -- full
// power assigned to it, nothing guessed at -- rather than picking a lethal
// amount that might be wrong in either direction.
func lethalDamage(target *Card, deathtouch bool) (int, bool) {
	if deathtouch {
		return 1, true
	}
	t, ok := target.Toughness()
	if !ok {
		return 0, false
	}
	remaining := t - target.Damage.Marked
	if remaining < 0 {
		remaining = 0
	}
	return remaining, true
}

// dealPermanentDamage marks amount on target -- Damage if it's a creature,
// loyalty or defense counters removed if it's a planeswalker or battle (CR
// 120.3c, 121.5) -- sourced from source, emits the DamageDealt event, and
// checks CR 603's own "whenever ~ deals damage" trigger
// (checkDamageDoneTriggersToCard, trigger.go). A permanent can be more than
// one of these (a creature planeswalker); each check runs independently
// rather than picking one, the same as Forge's own
// Card.addDamageAfterPrevention does.
func (g *Game) dealPermanentDamage(source, target CardID, amount int, deathtouch bool) {
	if amount <= 0 {
		return
	}
	c := g.Card(target)
	t := c.Type()
	if t.Has(cardtype.Planeswalker) {
		c.Counters.Add(Loyalty, -amount)
		emitCounterChanged(g.sink, source, CardEntity(target), Loyalty, -amount)
	}
	if t.Has(cardtype.Battle) {
		c.Counters.Add(Defense, -amount)
		emitCounterChanged(g.sink, source, CardEntity(target), Defense, -amount)
	}
	if t.Has(cardtype.Creature) {
		c.Damage.Mark(amount, deathtouch)
	}
	flags := FlagCombat
	if deathtouch {
		flags |= FlagDeathtouch
	}
	g.sink.Emit(Event{Kind: DamageDealt, Source: source, Target: CardEntity(target), Amount: int32(amount), Flags: flags})
	g.checkDamageDoneTriggersToCard(source, target, true)
}

// dealPlayerDamage reduces target's life by amount, emits DamageDealt and
// LifeChanged -- both, because Java's own combat damage step fires the
// equivalent of each separately and nothing downstream should have to derive
// one from the other -- and checks CR 603's own "whenever ~ deals damage"
// trigger (checkDamageDoneTriggersToPlayer, trigger.go), ValidTarget matched
// against a Player rather than a Card this time. Every call site already
// guards amount > 0 itself (unlike dealCreatureDamage, whose amount can come
// straight from an untrusted AssignCombatDamage answer), so there's nothing
// to re-check here.
func (g *Game) dealPlayerDamage(source CardID, target PlayerID, amount int) {
	g.Player(target).Life -= amount
	g.sink.Emit(Event{Kind: DamageDealt, Source: source, Target: PlayerEntity(target), Amount: int32(amount), Flags: FlagCombat})
	g.sink.Emit(Event{Kind: LifeChanged, Source: source, Target: PlayerEntity(target), Amount: int32(-amount), Flags: FlagCombat})
	g.checkDamageDoneTriggersToPlayer(source, target, true)
}
