// Event$ Moved replacements that act BEFORE a card enters the battlefield:
// the ones whose ReplaceWith$ chain moves the card itself (Lake of the Dead's
// "sacrifice a Swamp instead, if you do put it onto the battlefield, if you
// don't put it into its owner's graveyard", Containment Priest's "exile it
// instead", Captive Audience's "enters under the control of an opponent of
// your choice") and the "as it enters" ones that run first and let the move
// go on (Lich's ReplacementResult$ Updated LoseLife).
//
// Java runs ReplacementHandler.run(Moved) in GameAction.changeZone before the
// card leaves its zone; a Replaced result returns at once (GameAction.java:
// 334-365), so no zone change, no table entry and no ETB trigger happen for
// the entry that was replaced. The ChangeZone ability in the chain moves the
// card through a second, full changeZone, and ReplacementEffect.hasRun keeps
// the same replacement from applying to that nested move
// (ReplacementHandler.java:137, :227-228). Here every site that puts a card
// onto the battlefield (permanentEffect, attachEffect, playLandNow,
// moveByEffect) calls entryReplaced before Game.Move; runReplacements keeps
// the applied line in Game.replacing for the length of the chain, which is
// that hasRun.
//
// Layer order is ReplacementLayer.java:9-13: Control, then Copy ("enters as a
// copy", applyCopyReplacements in entersascopy.go), then Other. The Copy layer
// runs here too, before the move: a Clone entering under Containment Priest
// copies first and is then exiled as the copy, and the entering card's copy
// effect is dropped when the entry does not land (CR 400.7).

package engine

import (
	"strings"

	"github.com/jczastkiewicz/crucible/internal/carddb/compile"
	"github.com/jczastkiewicz/crucible/internal/expr"
	"github.com/jczastkiewicz/crucible/internal/valid"
)

// entryReplaced runs the pre-entry Moved replacements for moved about to
// enter the battlefield under entering from origin. It reports whether one
// replaced the entry: the caller then does not move the card, run the enter
// replacements or fire ETB triggers, because the replacement's own chain
// moved it (or did not) through a nested entry.
func (g *Game) entryReplaced(controller PlayerController, moved CardID, origin ZoneType, entering PlayerID) bool {
	card := g.Card(moved)
	if card.Def == nil {
		return false
	}
	// Java matches the entry on the card as it would exist on the battlefield,
	// controlled by whoever it enters under.
	savedController := card.controller
	card.controller = entering
	replaced := g.runEntryLayer(controller, card, origin, entering, "Control")
	if !replaced {
		// The Copy layer: the card takes its copied definition before the
		// move (CR 614.12), so the Other layer below and the move itself read
		// the copy, as ReplacementHandler.run's later layers do.
		g.refreshLayers()
		g.applyCopyReplacements(controller, moved, origin, entering)
		replaced = g.runEntryLayer(controller, card, origin, entering, "Other")
	}
	if replaced && card.Zone != Battlefield {
		// The entry did not land: a copy effect and the counters it was to
		// enter with go with the object that never entered (CR 400.7), not
		// onto the card in its new zone.
		if len(card.copies) > 0 {
			card.setCopies(nil)
		}
		card.pendingEnter = nil
	}
	if replaced && card.Zone != Battlefield {
		// A card the chain left outside the battlefield is not under the
		// entering player's control (CR 108.4a); Move resets it for a spell
		// leaving the stack and for the battlefield, not for a card in hand.
		if card.Zone != Stack {
			card.controller = savedController
		}
	} else if !replaced {
		card.controller = savedController
	}
	return replaced
}

// runEntryLayer runs one ReplacementLayer's pre-entry Moved replacements for
// card's entry (a runReplacements walk) and reports whether one replaced it.
func (g *Game) runEntryLayer(controller PlayerController, card *Card, origin ZoneType, entering PlayerID, layer string) bool {
	return g.runReplacements(controller, entering, func() []replacementCandidate {
		return g.entryCandidates(controller, card, origin, layer)
	}) == replacementReplaced
}

// entryCandidates lists the Event$ Moved lines of the given layer that apply
// to moved's entry and are resolvable here: moved's own (its faces and its
// perpetual grants, from whatever zone it is entering from), then every
// other trait host in play.
func (g *Game) entryCandidates(controller PlayerController, moved *Card, origin ZoneType, layer string) []replacementCandidate {
	var out []replacementCandidate
	consider := func(h *Card, z ZoneType, self bool, amounts map[string]expr.Amount, r *compile.Ability) {
		sub, updated, ok := g.entryShape(r, moved, origin, h, z, self, amounts, layer)
		if !ok {
			return
		}
		out = append(out, replacementCandidate{host: h, rule: r, apply: func() replacementResult {
			result := replacementReplaced
			if updated {
				result = replacementUpdated
			}
			if err := g.runReplacementChain(controller, h, amounts, sub, &replacementEvent{result: result, card: moved.ID}); err != nil {
				g.recordPendingError(err)
			}
			return result
		}})
	}
	for _, face := range moved.traitFaces() {
		for _, r := range face.Replacements {
			consider(moved, moved.Zone, true, face.Amounts, r)
		}
	}
	for _, grant := range moved.grants {
		for _, r := range grant.replacements {
			consider(moved, moved.Zone, true, grant.amounts, r)
		}
	}
	g.eachReplacementRule(func(h *Card, z ZoneType, amounts map[string]expr.Amount, r *compile.Ability) {
		if h.ID != moved.ID {
			consider(h, z, false, amounts, r)
		}
	})
	return out
}

// entryShape is whether r is a pre-entry Event$ Moved line this port runs for
// moved's entry, and its ReplaceWith$ ability. updated is the "as it enters"
// shape (ReplacementResult$ Updated): the ability runs and the entry
// continues. Otherwise the ability's chain must move the replaced card
// itself (a ChangeZone with Defined$ ReplacedCard), the shape that stands in
// for the entry.
func (g *Game) entryShape(r *compile.Ability, moved *Card, origin ZoneType, h *Card, hz ZoneType, self bool, amounts map[string]expr.Amount, layer string) (sub *compile.Ability, updated, ok bool) {
	if !strings.EqualFold(r.Name, "Moved") || !replacementZoneMatches(r, "Destination", Battlefield) || !replacementZoneMatches(r, "Origin", origin) {
		return nil, false, false
	}
	lineLayer, _ := r.Param("Layer")
	if lineLayer == "" {
		lineLayer = "Other"
	}
	if !strings.EqualFold(lineLayer, layer) {
		return nil, false, false
	}
	sub = replaceWithSub(r)
	if sub == nil {
		return nil, false, false
	}
	if v, has := r.Param("ReplacementResult"); has && strings.EqualFold(v, "Updated") {
		updated = true
		if !entryUpdatedShape(sub) {
			return nil, false, false
		}
	} else if !chainMovesReplacedCard(sub) {
		return nil, false, false
	}
	// A self line reads off the card's battlefield-bound copy whatever
	// ActiveZones$ says (getReplacementList's affectedLKI).
	if !self && !hostInActiveZones(h, r, hz) {
		return nil, false, false
	}
	validCard, has := r.Param("ValidCard")
	if !has || !Matches(g, moved, valid.Parse(validCard), h.Controller(), h.ID) {
		return nil, false, false
	}
	if !replacementRequirementsCheck(g, h, amounts, r) {
		return nil, false, false
	}
	return sub, updated, true
}

// chainMovesReplacedCard reports whether the ReplaceWith$ chain starting at
// sub contains a ChangeZone of Defined$ ReplacedCard: the ability that puts
// the replaced card somewhere in place of the entry.
func chainMovesReplacedCard(sub *compile.Ability) bool {
	for a := sub; a != nil; a = chainNext(a) {
		if strings.EqualFold(a.Name, "ChangeZone") {
			if d, ok := a.Param("Defined"); ok && strings.EqualFold(d, "ReplacedCard") {
				return true
			}
		}
	}
	return false
}

// chainNext is a's chained SubAbility$, nil when it has none.
func chainNext(a *compile.Ability) *compile.Ability {
	for _, sub := range a.Subs {
		if strings.EqualFold(sub.Key, "SubAbility") {
			return sub.Ability
		}
	}
	return nil
}

// entryUpdatedShape is the set of "as it enters" abilities that run before
// the move: a plain life loss (Lich), and the StoreSVar that remembers the
// life paid as the card enters (Phyrexian Processor, Minion of the Wastes,
// Nameless Race: payReplacementLifeX pays its Cost$).
func entryUpdatedShape(sub *compile.Ability) bool {
	if strings.EqualFold(sub.Name, "StoreSVar") {
		cost, _ := sub.Param("Cost")
		return cost == replacementPayLifeX
	}
	return strings.EqualFold(sub.Name, "LoseLife")
}
