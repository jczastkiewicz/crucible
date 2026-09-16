// Declaring blockers: CR 509.1, the step after declaring attackers.

package engine

import "github.com/jczastkiewicz/crucible/internal/cardtype"

// Blocks returns the current combat's blocking assignments, if any.
func (g *Game) Blocks() []Block { return g.combat.Blocks }

// DeclareCombatBlockers is CR 509.1: the defending player chooses which of their
// untapped creatures block which attacker. Unlike declaring attackers,
// blocking does not tap the blocker -- CR 508.1f only taps attackers, CR
// 509 has no equivalent.
//
// Eligibility here is only "untapped creature the defending player
// controls." Flying/reach (CR 702.9b), menace (CR 702.111b), protection,
// "can't be blocked except by," "must be blocked by," and every other CR
// 509.1b/509.1c restriction are not checked: in Forge, all of them (Flying
// included) run through the general CantBlockBy static-ability engine
// (StaticAbilityCantAttackBlock.java), not a keyword-specific check --
// unlike Indestructible or Vigilance, which Forge itself hardcodes
// (GameAction.java, Card.attackVigilance()) the same way HasKeyword does
// here. Reproducing Flying's restriction as a one-off HasKeyword check
// would invent a mechanism Forge doesn't use for it; the honest gap is
// "wait for the static-ability engine" (M5/M6), documented in
// game-state.md.
//
// "Who is defending" is each attacker's own defender (defenderOf,
// attack.go) -- the controller of whatever it's attacking, a player,
// planeswalker or battle. A two-player game, or a multiplayer game where
// the active player sent every attacker at one opponent, has exactly one:
// that defender is asked once, for every attacker, the same as before CR
// 506.4's multiplayer case existed. When attackers are split across more
// than one defending player at once, each defender is asked in turn, only
// about the attackers actually attacking them, offering only their own
// eligible creatures -- CR 509.1's "the defending player" read per
// defender rather than assumed singular. Defenders are asked in the order
// their first attacker appears in attackers, so the sequence is
// deterministic across a run (GO-12).
//
// A defender with no eligible creature is skipped, not asked with an
// empty list -- the same reasoning DeclareCombatAttackers uses for an
// active player with nothing to attack with. The combined result is nil,
// not an empty non-nil slice, when every defender is skipped this way.
func (g *Game) DeclareCombatBlockers(controller PlayerController) []Block {
	attackers := g.combat.Attackers
	if len(attackers) == 0 {
		return nil
	}

	byDefender := map[PlayerID][]CardID{}
	var defenders []PlayerID
	for _, id := range attackers {
		d := g.defenderOf(id)
		if _, ok := byDefender[d]; !ok {
			defenders = append(defenders, d)
		}
		byDefender[d] = append(byDefender[d], id)
	}

	var blocks []Block
	for _, defender := range defenders {
		var eligible []CardID
		for _, id := range g.Zone(Battlefield, defender).Cards() {
			c := g.Card(id)
			if c.Type().Has(cardtype.Creature) && !c.Tapped {
				eligible = append(eligible, id)
			}
		}
		if len(eligible) == 0 {
			continue
		}
		blocks = append(blocks, controller.DeclareCombatBlockers(g, defender, byDefender[defender], eligible)...)
	}
	g.combat.Blocks = blocks
	return blocks
}
