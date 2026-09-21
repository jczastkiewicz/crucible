// Declaring attackers: CR 508.1, the first step of combat this port
// reaches.

package engine

import "github.com/jczastkiewicz/crucible/internal/cardtype"

// Attackers returns the creatures currently attacking, if any.
func (g *Game) Attackers() []CardID { return g.combat.Attackers }

// DeclareCombatAttackers is CR 508.1: the active player chooses which of their
// eligible creatures attack. A creature is eligible if it is untapped and
// either has no summoning sickness or has haste (CR 302.6, CR 508.1a) --
// nothing this port can grant a creature the ability to attack tapped, so
// that half of 508.1a is not a case that can arise yet. Declared attackers
// tap, unless they have vigilance (CR 508.1f).
//
// If no creature is eligible, the controller is not asked at all: there is
// nothing meaningful to decide, the same reasoning a mulligan offer with no
// legal targets would have no question to ask either. This also keeps every
// existing scenario that walks through the DeclareAttackers phase (PhaseType,
// phase.go -- a different thing from this method, named after the same CR
// step) without ever creating a creature from needing to queue an empty
// answer.
//
// Not wired into AdvancePhase's automatic walk through the phases
// (turn.go): PerformMulligans is the precedent for a real M5 mechanic a
// scenario calls explicitly (actions.log's own `declareattackers` verb)
// rather than one the turn structure invokes unconditionally -- the same
// "stub standing in for a decision no one can make yet" reasoning that
// keeps ResolveStack out of beginPhase too, since most games reaching this
// phase attack with nothing and the call would be a no-op far more often
// than not.
// checkAttacksTriggers (trigger.go) runs once per declared attacker, after
// tapping and target assignment both landed -- CR 508.3's own "whenever ~
// attacks" trigger fires off the attack as declared, not off a
// still-provisional one. checkAttackersDeclaredTrigger (trigger.go) runs
// once after that loop, for CR 508.1's own "whenever a player attacks"
// trigger -- its own guard on an empty attackers slice is exactly why this
// method's own early return (below) never needs to call it at all.
func (g *Game) DeclareCombatAttackers(controller PlayerController) []CardID {
	var eligible []CardID
	for _, id := range g.Zone(Battlefield, g.activePlayer).Cards() {
		c := g.Card(id)
		if !c.Type().Has(cardtype.Creature) || c.Tapped {
			continue
		}
		if c.SummonSick && !c.HasKeyword("Haste") {
			continue
		}
		eligible = append(eligible, id)
	}
	if len(eligible) == 0 {
		return nil
	}

	attackers := controller.DeclareCombatAttackers(g, g.activePlayer, eligible)
	for _, id := range attackers {
		if !g.Card(id).HasKeyword("Vigilance") {
			g.Card(id).Tapped = true
			g.checkTapsTriggers(controller, id, g.Card(id).Controller(), true)
		}
	}
	g.combat.Attackers = attackers
	g.assignAttackTargets(controller, attackers)
	for _, id := range attackers {
		g.Card(id).AttacksThisTurn++
		g.checkAttacksTriggers(controller, id)
	}
	g.checkAttackersDeclaredOneTargetTrigger(controller)
	g.checkAttackersDeclaredTrigger(controller)
	return attackers
}

// AttackTarget returns what attacker is attacking -- a player, or a
// planeswalker/battle that player controls.
func (g *Game) AttackTarget(attacker CardID) EntityID { return g.combat.AttackTargets[attacker] }

// assignAttackTargets is CR 508.1d: for each declared attacker, what it's
// attacking. Every attacker shares the same eligible set (nothing this port
// models restricts one creature's targets differently from another's), so
// it's computed once and reused. A lone eligible target -- the ordinary
// two-player game with no planeswalker or battle on the other side -- is
// assigned automatically, the same "nothing meaningful to decide" reasoning
// DeclareCombatAttackers/Blockers use for an empty eligible list; more than
// one asks the controller per attacker (ChooseAttackTarget).
func (g *Game) assignAttackTargets(controller PlayerController, attackers []CardID) {
	eligible := g.eligibleAttackTargets()
	targets := make(map[CardID]EntityID, len(attackers))
	for _, id := range attackers {
		if len(eligible) == 1 {
			targets[id] = eligible[0]
			continue
		}
		targets[id] = controller.ChooseAttackTarget(g, g.activePlayer, id, eligible)
	}
	g.combat.AttackTargets = targets
}

// eligibleAttackTargets is every opponent still in the game, plus every
// planeswalker or battle any of them controls (CR 506.4c).
func (g *Game) eligibleAttackTargets() []EntityID {
	var eligible []EntityID
	for _, pid := range g.Players() {
		if pid == g.activePlayer || g.Player(pid).Lost {
			continue
		}
		eligible = append(eligible, PlayerEntity(pid))
		for _, id := range g.Zone(Battlefield, pid).Cards() {
			t := g.Card(id).Type()
			if t.Has(cardtype.Planeswalker) || t.Has(cardtype.Battle) {
				eligible = append(eligible, CardEntity(id))
			}
		}
	}
	return eligible
}

// defenderOf is the player defending against attacker: the player it's
// attacking directly, or the controller of the planeswalker/battle it's
// attacking (CR 802.4a's "attacking him/her or a planeswalker/battle he/she
// controls" is what makes that player the one who can block it).
//
// Called per attacker, not once for the whole combat: a two-player game, or
// a multiplayer game where the active player sent every attacker at a
// single opponent, gets the same defender back every time, but a combat
// split across more than one defending player at once (CR 506.4) does not,
// and DeclareCombatBlockers (block.go) groups by the result rather than
// assuming one answer for every attacker.
func (g *Game) defenderOf(attacker CardID) PlayerID {
	target := g.combat.AttackTargets[attacker]
	if pid, ok := target.AsPlayer(); ok {
		return pid
	}
	cid, _ := target.AsCard()
	return g.Card(cid).Controller()
}

// attackersOf is every creature currently attacking target directly -- a
// player or, more often the reason this exists, a planeswalker or battle
// (assignBattleProtector, action.go, CR 704.5w's "no attacking creatures
// currently attacking that battle"). Nil, not an error, when nothing is.
func (g *Game) attackersOf(target EntityID) []CardID {
	var attackers []CardID
	for _, id := range g.combat.Attackers {
		if g.combat.AttackTargets[id] == target {
			attackers = append(attackers, id)
		}
	}
	return attackers
}
