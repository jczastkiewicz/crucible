package engine

import (
	"fmt"
	"strings"

	"github.com/jczastkiewicz/crucible/internal/valid"
)

// destroyAllUnresolvedParams are DestroyAllEffect.java's params this port
// cannot honour yet. NoRegen$/NoRegenValid$ are accepted: regeneration is not
// ported, so every destroy already behaves as "can't be regenerated" -- the
// same reading destroyEffect gives NoRegen$.
var destroyAllUnresolvedParams = [...]string{
	"Optional", "RememberAllObjects", "Zone", "Hidden",
	"Condition", "ConditionDefined", "SorcerySpeed", "PlayerTurn", "ModeCost",
	"ActivationPhases", "GameActivationLimit",
}

// destroyAllEffect is DestroyAllEffect.java: every battlefield permanent
// matching ValidCards$ (all 238 real lines carry it), narrowed to the first
// targeted player's permanents when ValidTgts$ names one, is destroyed.
// Permanents that cannot be destroyed (canBeDestroyed, destroyeffect.go) are
// dropped first, as Java's own CardLists.filter(list, Card::canBeDestroyed).
//
// A ValidCards$ containing "X" is rejected: Java textually substitutes the
// X SVar's value into the valid string before parsing it, a runtime
// re-interpretation of script text PORT-2 forbids and this port has no
// compiled equivalent for yet.
type destroyAllEffect struct{}

func (destroyAllEffect) Resolve(g *Game, a *Ability, controller PlayerController) error {
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
		return fmt.Errorf("engine: DestroyAll: ValidCards$ %q names X, not resolvable yet", validCards)
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
	var destroyed []CardID
	for _, id := range list {
		c := g.Card(id)
		if c.Zone != Battlefield || !canBeDestroyed(c) {
			continue
		}
		g.Move(id, Graveyard, c.Owner)
		g.checkDiesTriggers(controller, id)
		if remember {
			source.Memory.Remember(CardEntity(id))
		}
		destroyed = append(destroyed, id)
	}
	g.checkChangesZoneAllTriggers(controller, destroyed, Battlefield, Graveyard)
	return nil
}
