// "As CARDNAME enters, it becomes ...": a Moved replacement whose
// ReplaceWith$ chain is RollDice then Animate of the entering card itself
// (Vedalken Squirrel-Whacker, Elvish Impersonators) or an Animate alone
// (Lydari Elephant, the Volvers' kicker keywords). Java runs the chain before
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

// entryAnimateChain reports whether sub's whole chain is RollDice
// (ResultSVar$ only) and Animate of Defined$ Self, ending in at least one
// Animate: the shape applyEnterAnimate runs.
func entryAnimateChain(sub *compile.Ability) bool {
	animates := 0
	for a, n := sub, 0; a != nil; a, n = chainNext(a), n+1 {
		if n > 32 || len(a.Subs) > 1 {
			return false
		}
		switch {
		case strings.EqualFold(a.Name, "Animate"):
			if d, ok := a.Param("Defined"); !ok || !strings.EqualFold(d, "Self") {
				return false
			}
			animates++
		case strings.EqualFold(a.Name, "RollDice"):
		default:
			return false
		}
	}
	return animates > 0
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
