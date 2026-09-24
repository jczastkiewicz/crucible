package engine

import (
	"fmt"

	"github.com/jczastkiewicz/crucible/internal/cardtype"
)

// changeCombatantsEffect is ChangeCombatantsEffect.java through
// SpellAbilityEffect.addToCombat with Attacking$ True (CR 506.3b): during
// combat, each Defined$ or targeted creature the attacking player controls
// becomes an attacker, attacking the defender the activator picks among
// every eligible one -- or, if it is already attacking, is redirected to
// it. Optional$ asks the activator per creature first. A defender set other
// than True (Attacking$ TargetedPlayer, ...) is not resolved.
type changeCombatantsEffect struct{}

func (changeCombatantsEffect) Resolve(g *Game, a *Ability, controller PlayerController) error {
	if err := rejectParams(a, "ChangeCombatants", "Condition", "TargetsWithDefinedController", "Blocking"); err != nil {
		return err
	}
	source := g.Card(a.Source)
	if !subAbilityConditionMet(g, source, a.Amounts, a.Params) {
		return nil
	}
	attacking, ok := a.Params.Param("Attacking")
	if !ok {
		return nil
	}
	if attacking != "True" {
		return fmt.Errorf("engine: ChangeCombatants: Attacking$ %q not resolvable yet", attacking)
	}
	cards, err := targetedOrDefinedCards(source, a.Params, a.refs())
	if err != nil {
		return fmt.Errorf("engine: ChangeCombatants: %w", err)
	}
	for _, id := range cards {
		c := g.Card(id)
		if !g.activePhase.IsCombat() || c.Zone != Battlefield || !c.Type().Has(cardtype.Creature) {
			continue
		}
		if c.Controller() != g.activePlayer {
			continue
		}
		if hasParam(a, "Optional") && !controller.ConfirmEffect(g, a.Controller, a.Source) {
			continue
		}
		eligible := g.eligibleAttackTargets()
		if len(eligible) == 0 {
			continue
		}
		defender := controller.ChooseAttackTarget(g, a.Controller, id, eligible)
		if err := checkChoice([]EntityID{defender}, eligible, 1, 1); err != nil {
			return fmt.Errorf("engine: ChangeCombatants: %w", err)
		}
		if g.combat.isAttacking(id) && g.combat.AttackTargets[id] == defender {
			continue
		}
		g.removeFromCombat(id)
		g.combat.Attackers = append(g.combat.Attackers, id)
		if g.combat.AttackTargets == nil {
			g.combat.AttackTargets = make(map[CardID]EntityID)
		}
		g.combat.AttackTargets[id] = defender
	}
	return nil
}
