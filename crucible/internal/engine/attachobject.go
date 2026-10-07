// AttachEffect's Object$ branch: attach named cards to a named host without
// a choice of host (Stolen Uniform).

package engine

//enginelint:allow id card game ability control defined zone condition staticability

import (
	"fmt"

	"github.com/jczastkiewicz/crucible/internal/cardtype"
)

// attachObject is AttachEffect.resolve's `Object$` branch without Choices$:
// each Object$ card is attached to the creature Defined$ (or a target) names
// (Stolen Uniform's "attach it to the chosen creature"). Java asks the
// chooser to pick among the host candidates; with one candidate that is no
// decision, and several are refused rather than guessed (GO-7). An
// attachment that left the battlefield is skipped; the equipment legality
// checks are the activated shape's (a creature host, Protection).
func (g *Game) attachObject(a *Ability, controller PlayerController) error {
	for _, key := range [...]string{"Choices", "PlayerChoices", "Optional", "Chooser", "Move"} {
		if _, ok := a.Params.Param(key); ok {
			return fmt.Errorf("engine: Attach: %s$ with Object$ not resolvable yet", key)
		}
	}
	source := g.Card(a.Source)
	if !subAbilityConditionMet(g, source, a.Amounts, a.Params) {
		return nil
	}
	raw, _ := a.Params.Param("Object")
	attachments, err := definedCards(source, raw, a.refs())
	if err != nil {
		return fmt.Errorf("engine: Attach: Object$: %w", err)
	}
	targets, err := targetedOrDefinedEntities(g, a)
	if err != nil {
		return err
	}
	if len(targets) == 0 {
		return nil
	}
	if len(targets) > 1 {
		return fmt.Errorf("engine: Attach: choosing among %d Defined$ hosts not resolvable yet", len(targets))
	}
	host, ok := targets[0].AsCard()
	if !ok || g.Card(host).Zone != Battlefield || !g.Card(host).Type().Has(cardtype.Creature) {
		return nil
	}
	for _, id := range attachments {
		if g.Card(id).Zone != Battlefield || id == host || hostRefusesAttach(g, g.Card(id), host) {
			continue
		}
		g.attachTo(controller, id, host)
	}
	return nil
}

// targetedOrDefinedEntities is getDefinedEntitiesOrTargeted(sa, "Defined"):
// the ability's targets when it has ValidTgts$, else its Defined$ entities
// (default Self).
func targetedOrDefinedEntities(g *Game, a *Ability) ([]EntityID, error) {
	if _, ok := a.Params.Param("ValidTgts"); ok {
		return a.Targets, nil
	}
	def, ok := a.Params.Param("Defined")
	if !ok {
		def = "Self"
	}
	es, err := definedEntities(g, a.Controller, g.Card(a.Source), def, a.refs())
	if err != nil {
		return nil, fmt.Errorf("engine: Attach: Defined$: %w", err)
	}
	return es, nil
}
