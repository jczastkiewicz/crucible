// Event$ TurnFaceUp and Event$ Transform: the replacements of a permanent
// changing face (CR 701.28, 702.37, 708.8). Java runs both after the state
// changed (Card.turnFaceUp, Card.changeCardState), so a ReplaceWith$ ability
// is not a substitute for the event but a "as it is turned face up" step
// that acts on the permanent already in its new state: Hooded Hydra's five
// counters, Zenos yae Galvus' "choose an opponent". Event$ TurnFaceUp with
// Layer$ CantHappen is the one gate in front: Card.canBeTurnedFaceUp.
//
// Ported from forge-game/src/main/java/forge/game/card/Card.java (turnFaceUp,
// canBeTurnedFaceUp, changeCardState) and
// replacement/{ReplaceTurnFaceUp,ReplaceTransform}.java.

package engine

import (
	"fmt"
	"strings"

	"github.com/jczastkiewicz/crucible/internal/carddb/compile"
	"github.com/jczastkiewicz/crucible/internal/expr"
	"github.com/jczastkiewicz/crucible/internal/valid"
)

// canBeTurnedFaceUp is Card.canBeTurnedFaceUp: no Layer$ CantHappen
// Event$ TurnFaceUp replacement whose ValidCard$ matches id (Unable to
// Scream's "enchanted creature can't be turned face up", Karlov Watchdog's
// "permanents your opponents control can't be turned face up during your
// turn") is in play and passing its requirements. A line with a param this
// port does not read records a pending error and does not block (GO-7).
func (g *Game) canBeTurnedFaceUp(id CardID) bool {
	card := g.Card(id)
	blocked := false
	g.eachReplacementRule(func(h *Card, z ZoneType, amounts map[string]expr.Amount, r *compile.Ability) {
		if blocked || !strings.EqualFold(r.Name, "TurnFaceUp") || !hostInActiveZones(h, r, z) || !replacementIsCantHappen(r) {
			return
		}
		if !onlyParams(r, "layer", "validcard") {
			g.recordPendingError(fmt.Errorf("engine: %q: Event$ TurnFaceUp: a param is not resolvable yet", h.Def.Name))
			return
		}
		if v, ok := r.Param("ValidCard"); ok && !Matches(g, card, valid.Parse(v), h.Controller(), h.ID) {
			return
		}
		if replacementRequirementsCheck(g, h, amounts, r) {
			blocked = true
		}
	})
	return !blocked
}

// faceChangeReplaced runs the ReplaceWith$ lines of event ("TurnFaceUp" or
// "Transform") for id, which already changed face: the CR 616 walk with the
// permanent's controller deciding. Each line's ReplaceWith$ ability, chain
// included, resolves as an ability of its host's controller with id as the
// replacing object (Defined$ ReplacedCard). The event cannot be stopped, so
// the walk's outcome is not read; a line whose ability errors records a
// pending error (GO-7).
func (g *Game) faceChangeReplaced(controller PlayerController, event string, id CardID) {
	card := g.Card(id)
	g.runReplacements(controller, card.Controller(), func() []replacementCandidate {
		var out []replacementCandidate
		g.eachReplacementRule(func(h *Card, z ZoneType, amounts map[string]expr.Amount, r *compile.Ability) {
			sub := replaceWithSub(r)
			if sub == nil || !strings.EqualFold(r.Name, event) || !hostInActiveZones(h, r, z) {
				return
			}
			if !onlyParams(r, "validcard", "layer", "optional", "optionaldecider", "replacementresult") {
				g.recordPendingError(fmt.Errorf("engine: %q: Event$ %s: a param is not resolvable yet", h.Def.Name, event))
				return
			}
			if v, ok := r.Param("ValidCard"); ok && !Matches(g, card, valid.Parse(v), h.Controller(), h.ID) {
				return
			}
			if !replacementRequirementsCheck(g, h, amounts, r) {
				return
			}
			out = append(out, replacementCandidate{host: h, rule: r, apply: func() replacementResult {
				if err := g.runReplacementChain(controller, h, amounts, sub, &replacementEvent{result: replacementReplaced, card: id}); err != nil {
					g.recordPendingError(err)
					return replacementNotReplaced
				}
				if v, _ := r.Param("ReplacementResult"); strings.EqualFold(v, "Updated") {
					return replacementUpdated
				}
				return replacementReplaced
			}})
		})
		return out
	})
}

// runReplacementChain resolves sub, a replacement's ReplaceWith$ ability,
// through the Registry as an ability of h's controller with ev as its
// replacing object, SubAbility$ chain included: ReplacementHandler plays the
// ability without the stack (playSpellAbilityNoStack), which resolves the
// chain with it. An error names the host and the API.
func (g *Game) runReplacementChain(controller PlayerController, h *Card, amounts map[string]expr.Amount, sub *compile.Ability, ev *replacementEvent) error {
	name := h.Def.Name
	api, ok := APIByName(sub.Name)
	if !ok {
		return fmt.Errorf("engine: %q: ReplaceWith$ names unknown API %q", name, sub.Name)
	}
	if g.registry == nil {
		return fmt.Errorf("engine: %q: ReplaceWith$ %s: no Registry on this Game", name, sub.Name)
	}
	a := Ability{API: api, Source: h.ID, Controller: h.Controller(), Params: sub, Amounts: amounts, replacing: ev}
	if err := g.registry.Resolve(g, &a, controller); err != nil {
		return fmt.Errorf("engine: %q: ReplaceWith$ %s: %w", name, sub.Name, err)
	}
	return nil
}

// destroyInstead is GameAction.destroy's Event$ Destroy replacement for id,
// which is about to be destroyed: a line with a ReplaceWith$ ability whose
// ValidCard$ matches id replaces the destruction with that ability (Harmonious
// Emergence and Crackling Emergence: "instead sacrifice CARDNAME and that land
// gains indestructible until end of turn"). The Regeneration$ True lines are
// regenerate's (regeneration.go). Reports whether a line replaced it.
func (g *Game) destroyInstead(controller PlayerController, id CardID) bool {
	card := g.Card(id)
	res := g.runReplacements(controller, card.Controller(), func() []replacementCandidate {
		var out []replacementCandidate
		g.eachReplacementRule(func(h *Card, z ZoneType, amounts map[string]expr.Amount, r *compile.Ability) {
			sub := replaceWithSub(r)
			if sub == nil || !strings.EqualFold(r.Name, "Destroy") || !hostInActiveZones(h, r, z) {
				return
			}
			if _, regen := r.Param("Regeneration"); regen {
				return
			}
			if !onlyParams(r, "validcard", "optional", "optionaldecider") {
				g.recordPendingError(fmt.Errorf("engine: %q: Event$ Destroy: a param is not resolvable yet", h.Def.Name))
				return
			}
			if v, ok := r.Param("ValidCard"); ok && !Matches(g, card, valid.Parse(v), h.Controller(), h.ID) {
				return
			}
			if !replacementRequirementsCheck(g, h, amounts, r) {
				return
			}
			out = append(out, replacementCandidate{host: h, rule: r, apply: func() replacementResult {
				if err := g.runReplacementChain(controller, h, amounts, sub, &replacementEvent{result: replacementReplaced, card: id}); err != nil {
					g.recordPendingError(err)
					return replacementNotReplaced
				}
				return replacementReplaced
			}})
		})
		return out
	})
	return res == replacementReplaced
}
