// UntapAll: CR 701.22, untapEffect's own batch-scan sibling and TapAll's
// own mirror image -- 105 real (AB|DB)$ UntapAll lines, all naming
// ValidCards$. Neither ValidTgts$ (1) nor Defined$ (3) present (101 of 105)
// scans every player's battlefield; either present scans only the named/
// targeted player's own battlefield instead -- UntapAllEffect.java's own
// identical `!sa.usesTargeting() && !sa.hasParam("Defined") ?
// game.getCardsIn(Battlefield) : getDefinedPlayersOrTargeted(sa).
// getCardsIn(Battlefield)`, ported the identical way tapalleffect.go's own
// is.
//
// Ported from
// forge-game/src/main/java/forge/game/ability/effects/UntapAllEffect.java's
// resolve. Not ported: TriggerType.UntapAll (the batch trigger) --
// checkUntapsTriggers already firing per card below covers every real
// corpus T: line, untapEffect.go's own identical reasoning for the same
// trigger reached from a script-driven Untap instead. ControllerUntaps$
// (Early Harvest), the "who untaps" attribution of the batch trigger, is not
// read: the only Mode$ UntapAll consumer (The Millennium Calendar) listens in
// the untap step, where no UntapAll effect can resolve (quirks.md).

package engine

//enginelint:allow id card game ability defined condition control zone parts valid trigger

import (
	"fmt"

	"github.com/jczastkiewicz/crucible/internal/valid"
)

// untapAllEffect resolves Mode$/DB$/AB$ UntapAll. ConditionPresent$/
// ConditionCompare$/ConditionCheckSVar$/ConditionSVarCompare$ are resolved
// through subAbilityConditionMet (condition.go), the identical way every
// other M6 effect's own does.
type untapAllEffect struct{}

func (untapAllEffect) Resolve(g *Game, a *Ability, controller PlayerController) error {
	if err := rejectUnreadCondition(a, a.Params.Name); err != nil {
		return err
	}
	source := g.Card(a.Source)
	if !subAbilityConditionMet(g, source, a.Amounts, a.Params) {
		return nil
	}

	validCards, ok := a.Params.Param("ValidCards")
	if !ok {
		return fmt.Errorf("engine: UntapAll: ValidCards$ missing")
	}
	spec := valid.Parse(validCards)

	_, hasValidTgts := a.Params.Param("ValidTgts")
	_, hasDefined := a.Params.Param("Defined")
	players := g.Players()
	if hasValidTgts || hasDefined {
		var err error
		players, err = targetedOrDefinedPlayers(g, a.Controller, a.Source, a.Params, a.refs())
		if err != nil {
			return fmt.Errorf("engine: UntapAll: %w", err)
		}
	}

	_, remUntapped := a.Params.Param("RememberUntapped")
	for _, pid := range players {
		for _, cid := range g.Zone(Battlefield, pid).Cards() {
			c := g.Card(cid)
			if !c.Tapped || !Matches(g, c, spec, a.Controller, a.Source) {
				continue
			}
			g.runUntapCommands(controller, cid)
			c.Tapped = false
			if remUntapped {
				source.Memory.Remember(CardEntity(cid))
			}
			g.checkUntapsTriggers(controller, cid)
		}
	}
	return nil
}
