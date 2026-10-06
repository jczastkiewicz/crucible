// The replacement Event$ values that resize or replace a player's own count:
// Scry (ScryNum, "Num"), Mill ("Number") and DrawCards ("Number"). Each is
// asked once per player before the action runs, with the count as the event's
// number, so a ReplaceEffect (Kenessos, Bruvac the Grandiloquent, Quantum
// Riddler) updates it and any other ReplaceWith$ ability (Eligeth's "draw that
// many cards instead", Alms Collector's "you and that player each draw a
// card") replaces the whole event.
//
// Ported from forge-game/src/main/java/forge/game/replacement/{ReplaceScry,
// ReplaceMill,ReplaceDrawCards}.java and the call sites in ScryEffect, MillEffect
// and Player.drawCards.

package engine

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/jczastkiewicz/crucible/internal/carddb/compile"
	"github.com/jczastkiewicz/crucible/internal/expr"
)

// playerAmountReplaced runs the CR 616 walk for event ("Scry", "Mill" or
// "DrawCards") about pid doing it amount times, amountName being the
// AbilityKey the number travels under ("Num" for Scry, "Number" otherwise).
// It returns the number to use and whether the event was replaced outright
// (the caller then does nothing). A line's ValidPlayer$ is matched against
// pid, Number$ ("GE2") against the amount; Optional$ is the handler's own
// confirmation (confirmOptionalReplacement). A param outside those records a
// pending error and skips the line (GO-7).
func (g *Game) playerAmountReplaced(controller PlayerController, event, amountName string, pid PlayerID, amount int) (int, bool) {
	res := g.runReplacements(controller, pid, func() []replacementCandidate {
		var out []replacementCandidate
		g.eachReplacementRule(func(h *Card, z ZoneType, amounts map[string]expr.Amount, r *compile.Ability) {
			sub := replaceWithSub(r)
			if sub == nil || !strings.EqualFold(r.Name, event) || !hostInActiveZones(h, r, z) {
				return
			}
			if !onlyParams(r, "validplayer", "number", "optional", "optionaldecider") {
				g.recordPendingError(fmt.Errorf("engine: %q: Event$ %s: a param is not resolvable yet", h.Def.Name, event))
				return
			}
			if v, ok := r.Param("ValidPlayer"); ok {
				matched, recognized := matchesPlayerSpec(g, pid, h.Controller(), h.ID, v)
				if !recognized || !matched {
					return
				}
			}
			if v, ok := r.Param("Number"); ok {
				n, err := strconv.Atoi(v[min(2, len(v)):])
				if len(v) < 3 || err != nil {
					g.recordPendingError(fmt.Errorf("engine: %q: Event$ %s: Number$ %q is not resolvable", h.Def.Name, event, v))
					return
				}
				if !compareOp(amount, v[:2], n) {
					return
				}
			}
			if !replacementRequirementsCheck(g, h, amounts, r) {
				return
			}
			out = append(out, replacementCandidate{host: h, rule: r, apply: func() replacementResult {
				ev := replacementEvent{amountName: amountName, amount: amount, player: pid}
				if api, ok := APIByName(sub.Name); ok {
					if _, isReplace := replaceEffectFor(api); isReplace {
						if !g.runReplaceWith(controller, h, amounts, sub, &ev) {
							g.recordPendingError(fmt.Errorf("engine: %q: Event$ %s: ReplaceWith$ %s is not resolvable yet", h.Def.Name, event, sub.Name))
							return replacementNotReplaced
						}
						amount = ev.amount
						return replacementUpdated
					}
				}
				ev.result = replacementReplaced
				if err := g.runReplacementChain(controller, h, replaceCountAmounts(g, amounts, h, sub, &ev), sub, &ev); err != nil {
					g.recordPendingError(err)
					return replacementNotReplaced
				}
				return replacementReplaced
			}})
		})
		return out
	})
	return amount, res == replacementReplaced
}

// replaceCountAmounts is amounts with every ReplaceCount$ expression the
// ReplaceWith$ ability sub (or a chained sub-ability) can read -- an SVar
// naming one, or one written inline in a param -- replaced by its value for
// ev, so an ordinary effect resolved through the Registry reads "the number
// that would have been" (Eligeth's NumCards$ X, Quantum Riddler's inline
// ReplaceCount$Number/Plus.1).
func replaceCountAmounts(g *Game, amounts map[string]expr.Amount, host *Card, sub *compile.Ability, ev *replacementEvent) map[string]expr.Amount {
	out := make(map[string]expr.Amount, len(amounts))
	for k, v := range amounts {
		out[k] = v
		if v.Kind == expr.Expression && strings.EqualFold(v.Head, "ReplaceCount") {
			if n, ok := resolveReplaceCountAmount(g, amounts, host, k, ev.amount, ev.amountName); ok {
				out[k] = expr.Amount{Kind: expr.Literal, Value: n}
			}
		}
	}
	for a := sub; a != nil; {
		for _, p := range a.Params {
			if !strings.HasPrefix(p.Value, "ReplaceCount$") {
				continue
			}
			if n, ok := resolveReplaceCountAmount(g, amounts, host, p.Value, ev.amount, ev.amountName); ok {
				out[strings.ToLower(p.Value)] = expr.Amount{Kind: expr.Literal, Value: n}
			}
		}
		var next *compile.Ability
		for _, s := range a.Subs {
			if strings.EqualFold(s.Key, "SubAbility") {
				next = s.Ability
			}
		}
		a = next
	}
	return out
}
