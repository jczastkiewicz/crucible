// "As CARDNAME enters, it becomes ...": a Moved replacement whose
// ReplaceWith$ chain acts on the entering card itself after some choices or
// dice: RollDice then Animate (Vedalken Squirrel-Whacker, Elvish
// Impersonators), an Animate alone (Lydari Elephant), an Attach (Grifter's
// Blade), a Reveal then "enters tapped unless" (Fortified Beachhead). Java runs the chain before
// the card lands and keeps its effect on the permanent; Game.Move rebuilds
// what an effect writes on a card that is still in its old zone, so this port
// runs the chain just after the move, before any ETB trigger or state-based
// action looks (enterBattlefieldReplacements). Nothing sees the permanent in
// between, so the order is not observable.

package engine

import (
	"strings"

	"github.com/jczastkiewicz/crucible/internal/carddb/compile"
	"github.com/jczastkiewicz/crucible/internal/expr"
	"github.com/jczastkiewicz/crucible/internal/valid"
)

// entryAnimateChain reports whether sub's whole chain is one applyEnterAnimate
// runs after the move: every node is either a before-move kind
// (entryChainAPIs: the choices, reveals, dice, cleanup around it) or one that
// acts on the entering card itself (Animate or Attach of Defined$/Object$
// Self, Tap with ETB$ of Defined$ Self), with at least one of the latter. A
// chain headed by Tap is checkMovedReplacement's. Sacrifice is left out when
// an Animate of Self follows: run after the move, the entering creature
// would be a legal sacrifice (Dracoplasm).
func entryAnimateChain(sub *compile.Ability) bool {
	if strings.EqualFold(sub.Name, "Tap") {
		return false
	}
	acts, sacrifices := 0, false
	for a, n := sub, 0; a != nil; a, n = chainNext(a), n+1 {
		if n > 32 || len(a.Subs) > 1 {
			return false
		}
		switch {
		case strings.EqualFold(a.Name, "Animate"):
			if d, ok := a.Param("Defined"); !ok || !strings.EqualFold(d, "Self") {
				return false
			}
			acts++
		case strings.EqualFold(a.Name, "Attach"):
			if d, ok := a.Param("Object"); !ok || !strings.EqualFold(d, "Self") {
				return false
			}
			acts++
		case strings.EqualFold(a.Name, "Tap"):
			if _, etb := a.Param("ETB"); !etb {
				return false
			}
			if d, ok := a.Param("Defined"); !ok || !strings.EqualFold(d, "Self") {
				return false
			}
			acts++
		default:
			known := false
			for _, api := range entryChainAPIs {
				known = known || strings.EqualFold(a.Name, api)
			}
			if !known {
				return false
			}
			sacrifices = sacrifices || strings.HasPrefix(strings.ToLower(a.Name), "sacrifice")
		}
	}
	return acts > 0 && (!sacrifices || !hasAnimateSelf(sub))
}

// hasAnimateSelf reports whether the chain from sub has an Animate node.
func hasAnimateSelf(sub *compile.Ability) bool {
	for a := sub; a != nil; a = chainNext(a) {
		if strings.EqualFold(a.Name, "Animate") {
			return true
		}
	}
	return false
}

// applyEnterAnimate runs moved's own "as it enters, it becomes" lines
// (K:ETBReplacement:Other with an entryAnimateChain), each once, after the
// move from origin. Another permanent's line is not read: no corpus line
// animates a different entering card this way.
func (g *Game) applyEnterAnimate(controller PlayerController, moved CardID, origin ZoneType) {
	card := g.Card(moved)
	if card.Def == nil {
		return
	}
	run := func(r *compile.Ability, amounts map[string]expr.Amount) {
		if !strings.EqualFold(r.Name, "Moved") || !replacementZoneMatches(r, "Destination", Battlefield) ||
			!replacementZoneMatches(r, "Origin", origin) {
			return
		}
		if v, has := r.Param("ReplacementResult"); !has || !strings.EqualFold(v, "Updated") {
			return
		}
		sub := replaceWithSub(r)
		if sub == nil || !entryAnimateChain(sub) {
			return
		}
		if validCard, ok := r.Param("ValidCard"); !ok || !Matches(g, card, valid.Parse(validCard), card.Controller(), card.ID) {
			return
		}
		if !replacementRequirementsCheck(g, card, amounts, r) {
			return
		}
		if err := g.runReplacementChain(controller, card, amounts, sub, &replacementEvent{result: replacementUpdated, card: moved}); err != nil {
			g.recordPendingError(err)
		}
	}
	for _, face := range card.traitFaces() {
		for _, r := range face.Replacements {
			run(r, face.Amounts)
		}
	}
	for _, grant := range card.grants {
		for _, r := range grant.replacements {
			run(r, grant.amounts)
		}
	}
}
