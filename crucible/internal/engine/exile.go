// Exiling a permanent as an activation cost: Exile<1/CARDNAME>, the identical
// self-reference shape Sac<1/CARDNAME> already has (ActivateAbility/
// ActivateManaAbility's own ActivationShape.SelfExile, internal/cost).
//
// Exile fires two trigger families. CR 603.6d's "leaves the battlefield"
// family (TriggerChangesZone.java's performTest, ported here the way
// checkDiesTriggers ports it for the graveyard) is checkExiledTriggers, which
// also runs when a Moved replacement redirects a move into exile. Mode$
// Exiled (TriggerExiled.java) is checkModeExiledTriggers: Java runs it only
// from GameAction.exile (GameAction.java:934-957), which every
// moveTo(ZoneType.Exile) reaches, and so does a Moved replacement's
// ReplaceWith$ ChangeZone into exile (Rest in Peace's).

package engine

import (
	"slices"
	"strings"

	"github.com/jczastkiewicz/crucible/internal/carddb/compile"
	"github.com/jczastkiewicz/crucible/internal/keyword"
	"github.com/jczastkiewicz/crucible/internal/valid"
)

// exileCards actually exiles each of ids -- GameAction.exile's own per-card
// loop (forge-game/src/main/java/forge/game/GameAction.java), mirroring
// sacrificeCards's own shape (sacrificeeffect.go) at the one real corpus
// need this cost primitive has: no RememberExiled$-equivalent to thread
// through (0 real Exile<1/CARDNAME> cost lines combine with one), so this
// takes only paying, the ability whose cost exiles ids (nil for an effect's
// exile, Meld), which Mode$ Exiled's WhileKeyword$ reads. Fires
// checkExiledTriggers and checkModeExiledTriggers per card (Move, game.go, already freezes g.LKI on any
// battlefield-leaving move, not only a move to the graveyard, so the
// dying-state lookback checkDiesTriggers relies on works here for free) and
// checkChangesZoneAllTriggers once for the whole batch, CR 603.6d's own
// batched trigger, reused wholesale exactly as sacrificeCards's own trailing
// call already is.
func exileCards(g *Game, controller PlayerController, paying *Ability, ids []CardID) {
	var exiled []CardID
	for _, id := range ids {
		c := g.Card(id)
		if c.Zone != Battlefield {
			continue
		}
		g.Move(id, Exile, c.Owner)
		g.checkExiledTriggers(controller, id)
		g.checkModeExiledTriggers(controller, id, Battlefield, paying)
		exiled = append(exiled, id)
	}
	g.checkChangesZoneAllTriggers(controller, exiled, Battlefield, Exile)
}

// isExiledTrigger reports whether t is CR 603.6d's "leaves the battlefield"
// shape restricted to the one destination checkExiledTriggers' own call
// sites ever reach, Exile -- isDiesTrigger's own exact sibling (trigger.go),
// Destination$ swapped from Graveyard to Exile. A line naming neither
// Destination$ Exile nor an unrestricted Destination$ (absent or "Any", CR
// 603.6c's own unqualified "leaves the battlefield," matched the identical
// way isDiesTrigger's own wildcard already is) does not match -- 35 real
// corpus lines name Destination$ with Exile in its list, most already
// permitting Battlefield as Origin$ too (absent, "Any", or a list naming it);
// every real "whenever ~ leaves the battlefield" line with no Destination$
// restriction at all is already counted in isDiesTrigger's own accounting
// and now correctly fires for an exile-caused departure too, not only a
// death.
func isExiledTrigger(t *compile.Ability) bool {
	return strings.EqualFold(t.Name, "ChangesZone") &&
		hasZoneOrAny(t, "Origin", Battlefield) &&
		hasZoneOrAny(t, "Destination", Exile) &&
		changesZoneResolvable(t)
}

// checkExiledTriggers is checkDiesTriggers' own exact structural sibling
// (trigger.go) -- own-card and other-watcher triggers both walked, g.LKI
// read for left's own dying-state fields the identical way, matches
// collected before pushTriggeredAbilities is called once -- with
// isExiledTrigger/Exile in every place checkDiesTriggers reads
// isDiesTrigger/Graveyard.
func (g *Game) checkExiledTriggers(controller PlayerController, left CardID) {
	var matches []Ability
	c := g.Card(left)
	if snap := g.LKI(left); snap != nil {
		c = snap
	}
	if c.Def != nil {
		for face := range c.triggerFaces {
			for _, t := range face.Triggers {
				if !isExiledTrigger(t) {
					continue
				}
				validCard, ok := t.Param("ValidCard")
				if !ok {
					continue
				}
				if !Matches(g, c, valid.Parse(validCard), c.Controller(), left) || g.triggerDisabled(c, t, c, Battlefield, Exile) {
					continue
				}
				// TriggerChangesZone.java:192-194: AbilityKey.Card (the pre-move
				// LKI) and NewCard (the exiled object) -- one CardID here.
				if sub, api, optional, ok := triggerEffectAPI(g, c, face.Amounts, t); ok {
					matches = append(matches, Ability{API: api, trigger: t, Source: left, Controller: c.Controller(), Params: sub, Amounts: face.Amounts, Optional: optional, triggered: face.objects(changesZoneCardObjects(t, left, c)), staticTrigger: isStaticTrigger(t)})
				}
			}
		}
	}
	matches = append(matches, g.otherExiledTriggerMatches(left)...)
	matches = append(matches, g.delayedLeftBattlefieldMatches(left, Exile)...)
	g.pushTriggeredAbilities(controller, matches)
}

// otherExiledTriggerMatches is otherDiesTriggerMatches' own exact sibling
// (trigger.go): every permanent still on the battlefield gets its own
// Triggers walked against left, the card that just left. No entered == left
// skip is needed for the identical reason otherDiesTriggerMatches' own has
// none: left is already in the exile zone by the time this runs.
func (g *Game) otherExiledTriggerMatches(left CardID) []Ability {
	var matches []Ability
	leaving := g.Card(left)
	if snap := g.LKI(left); snap != nil {
		leaving = snap
	}
	for _, pid := range g.Players() {
		for _, watcher := range g.Zone(Battlefield, pid).Cards() {
			w := g.Card(watcher)
			if w.Def == nil {
				continue
			}
			for face := range w.triggerFaces {
				for _, t := range face.Triggers {
					if !isExiledTrigger(t) {
						continue
					}
					validCard, ok := t.Param("ValidCard")
					if !ok {
						continue
					}
					if !Matches(g, leaving, valid.Parse(validCard), w.Controller(), watcher) || g.triggerDisabled(w, t, leaving, Battlefield, Exile) {
						continue
					}
					// TriggerChangesZone.java:192-194: AbilityKey.Card, the card
					// that left.
					if sub, api, optional, ok := triggerEffectAPI(g, w, face.Amounts, t); ok {
						matches = append(matches, Ability{API: api, trigger: t, Source: watcher, Controller: w.Controller(), Params: sub, Amounts: face.Amounts, Optional: optional, triggered: face.objects(changesZoneCardObjects(t, left, leaving)), staticTrigger: isStaticTrigger(t)})
					}
				}
			}
		}
	}
	return matches
}

// checkModeExiledTriggers is GameAction.exile's runTrigger(TriggerType.Exiled)
// (GameAction.java:934-957) for id, which just left origin for exile, matched
// by TriggerExiled.performTest (TriggerExiled.java:62-96) on every host whose
// TriggerZones$ admits its zone: Origin$ (a comma list, or Any), ValidCard$
// against id as it last was (AbilityKey.Card is the pre-move card), and
// WhileKeyword$ against paying, the ability whose cost is exiling id
// (Trigger.whileKeywordCheck reads the cost payment stack). A Madness line
// fires only from its own replacement (checkMadnessTriggers), and ValidCause$
// (0 corpus lines, a SpellAbility Matches cannot test) is not fired (GO-7).
// Java runs this from every origin; this port calls it for the battlefield
// only, the Origin$ every corpus line names.
func (g *Game) checkModeExiledTriggers(controller PlayerController, id CardID, origin ZoneType, paying *Ability) {
	left := g.Card(id)
	if snap := g.LKI(id); snap != nil && origin == Battlefield {
		left = snap
	}
	matches := g.scanTriggers([]string{"Exiled"}, func(h *Card, _ triggerFace, t *compile.Ability) (triggeredObjects, bool) {
		if t.Keyword != "" && keyword.Parse(t.Keyword).Name == "Madness" || hasAnyParam(t, "ValidCause") {
			return triggeredObjects{}, false
		}
		if o, ok := t.Param("Origin"); ok && o != "Any" && !slices.ContainsFunc(strings.Split(o, ","), func(name string) bool {
			z, ok := ZoneByName(name)
			return ok && z == origin
		}) {
			return triggeredObjects{}, false
		}
		if !g.triggerCardMatches(h, t, "ValidCard", left) {
			return triggeredObjects{}, false
		}
		if w, ok := t.Param("WhileKeyword"); ok && !whileKeywordMatches(w, paying) {
			return triggeredObjects{}, false
		}
		objs := triggeredObjects{card: id}
		if origin == Battlefield {
			objs = objs.withCardLKI(left)
		}
		return objs, true
	})
	g.pushTriggeredAbilities(controller, matches)
}

// whileKeywordMatches is Trigger.whileKeywordCheck (Trigger.java:644-656) for
// the one shape the corpus's Mode$ Exiled line names, Ability.<keyword>: the
// paying ability is an activated ability made from that keyword (Market
// Gnome's Ability.Craft). Any other shape does not match (GO-7).
func whileKeywordMatches(spec string, paying *Ability) bool {
	kw, ok := strings.CutPrefix(spec, "Ability.")
	if !ok || paying == nil || paying.spell || paying.Params == nil || paying.Params.Keyword == "" {
		return false
	}
	return keyword.Parse(paying.Params.Keyword).Name == kw
}
