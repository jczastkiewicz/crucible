// The regular combat damage step: CR 510.1-510.3.

package engine

// DealCombatDamage is CR 510.1-510.3: every attacker and every creature
// blocking it deals combat damage. Not implemented: the first strike
// sub-step (CR 510.4, 702.7 -- nothing this port has checked "First Strike"
// anywhere yet) and trample (CR 702.19 -- a blocked attacker never deals
// excess damage to a player here, whether or not it has the keyword).
// Deathtouch is checked: any 1 damage from a deathtouch source is lethal,
// same "the game decides what is legal" question `destroyDamagedCreatures`
// already answers for the state-based action this feeds.
//
// All of combat's damage is simultaneous (CR 510.2), but nothing yet
// triggers off damage being dealt (no lifelink, no "whenever this deals
// damage" ability) or cares about the order two life totals change in, so
// applying one attacker's exchange at a time produces the same result as
// computing every amount first and applying them together.
//
// An unblocked attacker deals its power to the player it's attacking --
// nextPlayerAfter's own single-defender assumption, the same gap
// DeclareCombatBlockers already carries (game-state.md). A blocked attacker
// with exactly one blocker exchanges full power for full power
// automatically, no decision needed, the same "nothing meaningful to
// decide" reasoning DeclareCombatAttackers/Blockers use for an empty
// eligible list. A gang-blocked attacker (more than one Block naming it)
// asks the attacking player how to divide its power among them
// (AssignCombatDamage, CR 510.1c).
//
// A creature that stops being on the battlefield between DeclareCombatBlockers
// and this step (removed by something the game asked a question about) would
// need CR 510.1c's "attacker remains blocked but the removed blocker gets
// nothing" exception; nothing built yet can remove a creature in that
// window, so the case cannot arise and isn't handled.
func (g *Game) DealCombatDamage(controller PlayerController) {
	blockersOf := make(map[CardID][]CardID, len(g.combat.Blocks))
	for _, b := range g.combat.Blocks {
		blockersOf[b.Attacker] = append(blockersOf[b.Attacker], b.Blocker)
	}

	for _, atkID := range g.combat.Attackers {
		atk := g.Card(atkID)
		power, ok := atk.Power()
		blockers := blockersOf[atkID]
		deathtouch := atk.HasKeyword("Deathtouch")

		if ok && power > 0 {
			switch len(blockers) {
			case 0:
				defender := g.nextPlayerAfter(g.activePlayer)
				g.dealPlayerDamage(atkID, defender, power)
			case 1:
				g.dealCreatureDamage(atkID, blockers[0], power, deathtouch)
			default:
				for _, a := range controller.AssignCombatDamage(g, atk.Controller, atkID, blockers) {
					g.dealCreatureDamage(atkID, a.Blocker, a.Amount, deathtouch)
				}
			}
		}

		for _, blkID := range blockers {
			blk := g.Card(blkID)
			if bp, ok := blk.Power(); ok && bp > 0 {
				g.dealCreatureDamage(blkID, atkID, bp, blk.HasKeyword("Deathtouch"))
			}
		}
	}
}

// dealCreatureDamage marks amount on target, sourced from source, and emits
// the DamageDealt event.
func (g *Game) dealCreatureDamage(source, target CardID, amount int, deathtouch bool) {
	g.Card(target).Damage.Mark(amount, deathtouch)
	flags := FlagCombat
	if deathtouch {
		flags |= FlagDeathtouch
	}
	g.sink.Emit(Event{Kind: DamageDealt, Source: source, Target: CardEntity(target), Amount: int32(amount), Flags: flags})
}

// dealPlayerDamage reduces target's life by amount and emits DamageDealt and
// LifeChanged -- both, because Java's own combat damage step fires the
// equivalent of each separately and nothing downstream should have to derive
// one from the other.
func (g *Game) dealPlayerDamage(source CardID, target PlayerID, amount int) {
	g.Player(target).Life -= amount
	g.sink.Emit(Event{Kind: DamageDealt, Source: source, Target: PlayerEntity(target), Amount: int32(amount), Flags: FlagCombat})
	g.sink.Emit(Event{Kind: LifeChanged, Source: source, Target: PlayerEntity(target), Amount: int32(-amount), Flags: FlagCombat})
}
