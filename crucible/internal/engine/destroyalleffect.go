package engine

//enginelint:allow id card game ability amount condition control parts zone valid destroyeffect

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/jczastkiewicz/crucible/internal/valid"
)

// destroyAllUnresolvedParams are DestroyAllEffect.java's params this port
// cannot honour yet. NoRegen$ and NoRegenValid$ (the per-card version) stop a
// regeneration shield (regeneration.go) from replacing the destruction.
var destroyAllUnresolvedParams = [...]string{
	"Optional", "RememberAllObjects", "Zone", "Hidden",
}

// destroyAllEffect is DestroyAllEffect.java: every battlefield permanent
// matching ValidCards$ (all 238 real lines carry it), narrowed to the first
// targeted player's permanents when ValidTgts$ names one, is destroyed.
// Permanents that cannot be destroyed (canBeDestroyed, destroyeffect.go) are
// dropped first, as Java's own CardLists.filter(list, Card::canBeDestroyed).
//
// A ValidCards$ containing "X" has every X replaced by the value of the X
// SVar before it is parsed, as Java does (DestroyAllEffect.java:57-60), so
// an X anywhere in the string, a name's included, is replaced.
type destroyAllEffect struct{}

func (destroyAllEffect) Resolve(g *Game, a *Ability, controller PlayerController) error {
	if err := rejectUnreadCondition(a, a.Params.Name); err != nil {
		return err
	}
	for _, key := range destroyAllUnresolvedParams {
		if _, ok := a.Params.Param(key); ok {
			return fmt.Errorf("engine: DestroyAll: %s$ not resolvable yet", key)
		}
	}
	source := g.Card(a.Source)
	if !subAbilityConditionMet(g, source, a.Amounts, a.Params) {
		return nil
	}
	validCards, _ := a.Params.Param("ValidCards")
	if strings.Contains(validCards, "X") {
		x, ok := resolveNamedAmount(g, a.Amounts, source, "X")
		if !ok {
			return fmt.Errorf("engine: DestroyAll: ValidCards$ %q: SVar X is not resolvable", validCards)
		}
		validCards = strings.ReplaceAll(validCards, "X", strconv.Itoa(x))
	}
	spec := valid.Parse(validCards)

	targetPlayer := NoPlayer
	for _, e := range a.Targets {
		if pid, ok := e.AsPlayer(); ok {
			targetPlayer = pid
			break
		}
	}

	var list []CardID
	for _, pid := range g.Players() {
		for _, cid := range g.Zone(Battlefield, pid).Cards() {
			c := g.Card(cid)
			if targetPlayer != NoPlayer && c.Controller() != targetPlayer {
				continue
			}
			if validCards == "" || Matches(g, c, spec, a.Controller, a.Source) {
				list = append(list, cid)
			}
		}
	}

	_, remember := a.Params.Param("RememberDestroyed")
	if remember {
		source.Memory.ClearRemembered()
	}
	_, noRegen := a.Params.Param("NoRegen")
	var noRegenSpec valid.Spec
	noRegenValid, hasNoRegenValid := a.Params.Param("NoRegenValid")
	if hasNoRegenValid {
		noRegenSpec = valid.Parse(noRegenValid)
	}
	var destroyed []CardID
	for _, id := range list {
		c := g.Card(id)
		if c.Zone != Battlefield || !canBeDestroyed(c) {
			continue
		}
		cantRegen := noRegen
		if hasNoRegenValid {
			cantRegen = Matches(g, c, noRegenSpec, a.Controller, a.Source)
		}
		if g.destroyInstead(controller, id) || (!cantRegen && g.regenerate(controller, id)) {
			continue
		}
		g.moveToGraveyard(controller, id)
		g.checkDiesTriggers(controller, id)
		if remember {
			source.Memory.Remember(CardEntity(id))
		}
		destroyed = append(destroyed, id)
	}
	g.checkChangesZoneAllTriggers(controller, destroyed, Battlefield, Graveyard)
	return nil
}
