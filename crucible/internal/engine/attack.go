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
		}
	}
	g.combat.Attackers = attackers
	return attackers
}
