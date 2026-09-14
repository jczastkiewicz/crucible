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
// "Who is defending" is attackers[0]'s own defender (defenderOf, attack.go)
// -- the controller of whatever it's attacking, a player, planeswalker or
// battle. Every attacker in this combat is assumed to share that one
// defender, so only the first is asked; a combat where the active player
// split attackers across several different defending players at once isn't
// handled -- defenderOf's own doc comment has the reason why.
//
// If no creature is eligible, the controller is not asked at all, the same
// reasoning DeclareCombatAttackers uses for an active player with nothing
// to attack with.
func (g *Game) DeclareCombatBlockers(controller PlayerController) []Block {
	attackers := g.combat.Attackers
	if len(attackers) == 0 {
		return nil
	}
	defender := g.defenderOf(attackers[0])

	var eligible []CardID
	for _, id := range g.Zone(Battlefield, defender).Cards() {
		c := g.Card(id)
		if c.Type().Has(cardtype.Creature) && !c.Tapped {
			eligible = append(eligible, id)
		}
	}
	if len(eligible) == 0 {
		return nil
	}

	blocks := controller.DeclareCombatBlockers(g, defender, attackers, eligible)
	g.combat.Blocks = blocks
	return blocks
}
