// The cause of a zone move: AbilityKey.Cause of TriggerChangesZone and
// TriggerChangesZoneAll (GameAction.java:605, CardZoneTable.java:100-133),
// read by ValidCause$, InvertValidCause$ and NotThisAbility$.

package engine

//enginelint:allow game ability card trigger saspec id

import (
	"github.com/jczastkiewicz/crucible/internal/carddb/compile"
	"github.com/jczastkiewicz/crucible/internal/expr"
)

// withMoveCause makes a the cause of every zone move until the returned
// restore runs: the ability whose cost is being paid, the stand-in of a land
// play (landAbility), or nil for a state-based action. Outside it a move's
// cause is the ability resolving (Registry.resolve), as Java passes the
// effect's SpellAbility to moveTo.
func (g *Game) withMoveCause(a *Ability) func() {
	prev, prevSet := g.moveCause, g.moveCauseSet
	g.moveCause, g.moveCauseSet = a, true
	return func() { g.moveCause, g.moveCauseSet = prev, prevSet }
}

// landPlayCause makes a land play the cause of its moves (LandAbility): pid
// playing card.
func (g *Game) landPlayCause(card CardID, pid PlayerID) func() {
	return g.withMoveCause(&Ability{Source: card, Controller: pid, landAbility: true})
}

// currentMoveCause is the cause of a zone move happening now.
func (g *Game) currentMoveCause() *Ability {
	if g.moveCauseSet {
		return g.moveCause
	}
	return g.resolving
}

// zoneCauseMet is TriggerChangesZone's ValidCause$/InvertValidCause$
// (:147, CardTraitBase.matchesValidParam's Invert, CardTraitBase.java:
// 260-266) and NotThisAbility$ (:161-168) against the move happening now.
// ValidCause$ reads the cause through spellAbilityMatches against the
// trigger host: Sacred Ground's SpellAbility.OppCtrl, the YouCtrl lines,
// Deep Gnome Terramancer's LandAbility inverted. No cause matches nothing,
// so an inverted line matches it; a spec this port cannot read does not
// match either way (GO-7). NotThisAbility$ fails when the move is the
// trigger's own resolving ability (Kodama of the East Tree).
func (g *Game) zoneCauseMet(host *Card, amounts map[string]expr.Amount, t *compile.Ability) bool {
	cause := g.currentMoveCause()
	if _, ok := t.Param("NotThisAbility"); ok && cause != nil && cause.trigger == t {
		return false
	}
	spec, ok := t.Param("ValidCause")
	if !ok {
		return true
	}
	matched := false
	if cause != nil && cause.Source != NoCard {
		m, known := g.spellAbilityMatches(cause, spec, host, host.Controller(), amounts)
		if !known {
			return false
		}
		matched = m
	}
	if _, invert := t.Param("InvertValidCause"); invert {
		return !matched
	}
	return matched
}
