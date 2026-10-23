// Event$ RemoveCounter (ReplaceRemoveCounter.java), the replacement of a card
// losing counters: Fear of Sleep Paralysis's "can't remove stun counters from
// permanents your opponents control" (Layer$ CantHappen) and Deification's
// "reduces it to 1 instead" for loyalty lost to damage. 2 real corpus lines.
//
// Ported from forge-game/src/main/java/forge/game/card/Card.java
// canRemoveCounters (:1737-1749) and subtractCounter (:1886-1908): the
// CantHappen lines are asked first with Result 0, then the remaining counters
// are offered to the other lines as Result.

package engine

//enginelint:allow id card game ability defined condition control parts event amount replacement valid replaceeffect

import (
	"strings"

	"github.com/jczastkiewicz/crucible/internal/carddb/compile"
	"github.com/jczastkiewicz/crucible/internal/expr"
	"github.com/jczastkiewicz/crucible/internal/valid"
)

// removeCounterLineMatches checks the params every Event$ RemoveCounter line
// reads (ReplaceRemoveCounter.canReplace, :28-54) against card losing kind with
// result counters left: ValidCard$, IsDamage$, ValidCounterType$ (a string
// compare) and Result$ (a two-letter operator over an amount). ok is false for
// a line naming something else, which is skipped, never half applied (GO-7).
func (g *Game) removeCounterLineMatches(h *Card, amounts map[string]expr.Amount, r *compile.Ability, card CardID, kind CounterType, result int, isDamage bool) (matched, ok bool) {
	if !onlyParams(r, "validcard", "validcountertype", "isdamage", "result", "layer") {
		return false, false
	}
	if v, has := r.Param("ValidCard"); has && !Matches(g, g.Card(card), valid.Parse(v), h.Controller(), h.ID) {
		return false, true
	}
	if v, has := r.Param("IsDamage"); has && strings.EqualFold(v, "True") != isDamage {
		return false, true
	}
	if v, has := r.Param("ValidCounterType"); has && v != string(kind) {
		return false, true
	}
	if v, has := r.Param("Result"); has {
		if len(v) < 3 {
			return false, false
		}
		operand, resolved := resolveNamedAmount(g, amounts, h, v[2:])
		if !resolved {
			return false, false
		}
		switch strings.ToUpper(v[:2]) {
		case "LT":
			return result < operand, true
		case "LE":
			return result <= operand, true
		case "GT":
			return result > operand, true
		case "GE":
			return result >= operand, true
		case "EQ":
			return result == operand, true
		case "NE":
			return result != operand, true
		}
		return false, false
	}
	return true, true
}

// removeCounterBlocked is Card.canRemoveCounters for kind on card: a Layer$
// CantHappen line with no ReplaceWith$ stops it. Result$ is read as 0
// (Card.java:1745).
func (g *Game) removeCounterBlocked(card CardID, kind CounterType) bool {
	blocked := false
	g.eachReplacement("RemoveCounter", func(h *Card, amounts map[string]expr.Amount, r *compile.Ability) bool {
		if !replacementIsCantHappen(r) {
			return false
		}
		if m, ok := g.removeCounterLineMatches(h, amounts, r, card, kind, 0, false); ok && m && replacementRequirementsCheck(g, h, amounts, r) {
			blocked = true
			return true
		}
		return false
	})
	return blocked
}

// removeCounterReplaced offers the counters card keeps (result) after losing
// kind to the Event$ RemoveCounter lines with a ReplaceWith$ (Card.java:1894):
// ReplaceEffect's Result$ becomes what stays. isDamage is a planeswalker or
// battle losing counters to damage. The first line that applies decides,
// as in the other event dispatches.
func (g *Game) removeCounterReplaced(controller PlayerController, card CardID, kind CounterType, result int, isDamage bool) int {
	g.eachReplacement("RemoveCounter", func(h *Card, amounts map[string]expr.Amount, r *compile.Ability) bool {
		sub := replaceWithSub(r)
		if sub == nil {
			return false
		}
		if m, ok := g.removeCounterLineMatches(h, amounts, r, card, kind, result, isDamage); !ok || !m || !replacementRequirementsCheck(g, h, amounts, r) {
			return false
		}
		ev := replacementEvent{amountName: "Result", amount: result, counterType: kind}
		if !g.runReplaceWith(controller, h, amounts, sub, &ev) {
			return false
		}
		result = ev.amount
		return true
	})
	return result
}
