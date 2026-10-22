// Mode$ RolledDie and RolledDieOnce, fired by RollDice
// (RollDiceEffect.java:320-336): one RolledDie per die kept, then one
// RolledDieOnce with every kept result. The planar die has its own refusal
// (rollplanardiceeffect.go).

package engine

//enginelint:allow id card ability trigger triggerscan triggerpack staticability amount game control valid

import (
	"strconv"
	"strings"

	"github.com/jczastkiewicz/crucible/internal/carddb/compile"
)

var (
	rolledDieParams     = effectEventParams("validplayer", "validresult", "validsides", "number", "natural", "rolledtovisitattractions")
	rolledDieOnceParams = effectEventParams("validplayer", "rolledtovisitattractions")
)

// rolledDieTriggerSupported reports whether this port reads every param of t.
func rolledDieTriggerSupported(t *compile.Ability) bool {
	if strings.EqualFold(t.Name, "RolledDieOnce") {
		return paramsResolvable(t, rolledDieOnceParams)
	}
	return paramsResolvable(t, rolledDieParams)
}

// checkRolledDieTriggers runs RollDiceEffect.java:320-336 for one player's
// roll: natural and modified are the kept dice (after IgnoreLower$), rolled
// is how many were rolled (Number$ counts from the player's running total
// minus them). RolledToVisitAttractions$ lines never fire: an ordinary roll
// carries the flag false (TriggerRolledDie.java:27-30, presence only).
func (g *Game) checkRolledDieTriggers(controller PlayerController, pid PlayerID, sides, rolled int, natural, modified []int) {
	var matches []Ability
	for i := range modified {
		number := g.Player(pid).RollsThisTurn - rolled + i + 1
		result, nat := modified[i], natural[i]
		matches = append(matches, g.scanTriggers([]string{"RolledDie"}, func(h *Card, _ triggerFace, t *compile.Ability) (triggeredObjects, bool) {
			if !rolledDieTriggerSupported(t) || hasTriggerParam(t, "RolledToVisitAttractions") ||
				!g.triggerPlayerMatches(h, t, "ValidPlayer", pid) {
				return triggeredObjects{}, false
			}
			if v, ok := t.Param("ValidSides"); ok && !rolledDieNumberIn(v, sides) {
				return triggeredObjects{}, false
			}
			if v, ok := t.Param("Number"); ok && !rolledDieNumberIn(v, number) {
				return triggeredObjects{}, false
			}
			if v, ok := t.Param("ValidResult"); ok {
				seen := result
				if hasTriggerParam(t, "Natural") {
					seen = nat
				}
				if !rolledDieResultValid(v, seen, sides) {
					return triggeredObjects{}, false
				}
			}
			return triggeredObjects{player: pid, counts: triggerCounts{results: []int{result}, set: countResult}}, true
		})...)
	}
	matches = append(matches, g.scanTriggers([]string{"RolledDieOnce"}, func(h *Card, _ triggerFace, t *compile.Ability) (triggeredObjects, bool) {
		if !rolledDieTriggerSupported(t) || hasTriggerParam(t, "RolledToVisitAttractions") ||
			!g.triggerPlayerMatches(h, t, "ValidPlayer", pid) {
			return triggeredObjects{}, false
		}
		return triggeredObjects{player: pid, counts: triggerCounts{results: append([]int(nil), modified...), set: countResult}}, true
	})...)
	g.pushTriggeredAbilities(controller, matches)
}

func hasTriggerParam(t *compile.Ability, key string) bool {
	_, ok := t.Param(key)
	return ok
}

// rolledDieNumberIn is TriggerRolledDie's ValidSides$ and Number$ tests
// (:34-44): the value is one of a comma list of integers.
func rolledDieNumberIn(spec string, n int) bool {
	for _, part := range strings.Split(spec, ",") {
		if v, err := strconv.Atoi(strings.TrimSpace(part)); err == nil && v == n {
			return true
		}
	}
	return false
}

// rolledDieResultValid is TriggerRolledDie.isResultValid (:64-92): a number
// is an exact roll, Highest is the die's side count, and a two-letter
// comparator with an operand (GE4) compares.
func rolledDieResultValid(spec string, result, sides int) bool {
	for _, part := range strings.Split(spec, ",") {
		switch {
		case part == "Highest":
			if result == sides {
				return true
			}
		case len(part) > 2 && !isDigits(part):
			if right, err := strconv.Atoi(part[2:]); err == nil && javaCompare(result, part[:2], right) {
				return true
			}
		default:
			if v, err := strconv.Atoi(part); err == nil && v == result {
				return true
			}
		}
	}
	return false
}

func isDigits(s string) bool {
	_, err := strconv.Atoi(s)
	return err == nil
}
