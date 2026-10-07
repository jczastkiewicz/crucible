// A permanent leaving the battlefield for its owner's hand or library: the
// "leaves the battlefield" ChangesZone triggers of CR 603.6d at those two
// destinations, checkDiesTriggers' and checkExiledTriggers' sibling, plus the
// live delayed ChangesZone watches registered for the card (a delayed
// "when it leaves the battlefield" naming Destination$ Any, Hand or Library).
//
// Ported from forge-game's TriggerChangesZone.performTest and the zone-change
// run in GameAction.changeZone; this port keeps one function per destination
// family, so a bounce and a tuck share this file and its dies/exile
// siblings stay where they were.

package engine

//enginelint:allow id card game ability control trigger statictrigger valid zone

import (
	"strings"

	"github.com/jczastkiewicz/crucible/internal/carddb/compile"
	"github.com/jczastkiewicz/crucible/internal/valid"
)

// isLeftToTrigger reports whether t is CR 603.6d's "leaves the battlefield"
// shape restricted to destination dest: Mode$ ChangesZone, Origin$ naming (or
// not restricting away) the battlefield, Destination$ naming (or not
// restricting away) dest.
func isLeftToTrigger(t *compile.Ability, dest ZoneType) bool {
	return strings.EqualFold(t.Name, "ChangesZone") &&
		hasZoneOrAny(t, "Origin", Battlefield) &&
		hasZoneOrAny(t, "Destination", dest) &&
		changesZoneResolvable(t)
}

// checkLeftToTriggers fires the leaves-the-battlefield triggers of left, a
// permanent that just moved to dest (Hand or Library): its own Card.Self
// triggers, the triggers of every permanent still on the battlefield and the
// live delayed watches. left's last-known information is read for the
// dying-state fields, as checkDiesTriggers does.
func (g *Game) checkLeftToTriggers(controller PlayerController, left CardID, dest ZoneType) {
	var matches []Ability
	c := g.Card(left)
	if snap := g.LKI(left); snap != nil {
		c = snap
	}
	if c.Def != nil {
		for face := range c.triggerFaces {
			for _, t := range face.Triggers {
				if !isLeftToTrigger(t, dest) {
					continue
				}
				validCard, ok := t.Param("ValidCard")
				if !ok {
					continue
				}
				if !Matches(g, c, valid.Parse(validCard), c.Controller(), left) {
					continue
				}
				if sub, api, optional, ok := triggerEffectAPI(g, c, face.Amounts, t); ok {
					matches = append(matches, Ability{API: api, Source: left, Controller: c.Controller(), Params: sub, Amounts: face.Amounts, Optional: optional, triggered: face.objects(triggeredObjects{card: left}), staticTrigger: isStaticTrigger(t)})
				}
			}
		}
	}
	matches = append(matches, g.otherLeftToTriggerMatches(left, dest)...)
	matches = append(matches, g.delayedLeftBattlefieldMatches(left, dest)...)
	g.pushTriggeredAbilities(controller, matches)
}

// checkLeftGraveyardTriggers fires the ChangesZone triggers naming a
// graveyard origin (Krovikan Vampire's "Origin$ Graveyard | Destination$ Any |
// ValidCard$ Card.IsRemembered") for card, which a resolving effect just moved
// from its owner's graveyard to dest. dest is never the battlefield: the
// enters-the-battlefield walk (checkETBTriggers) already reads Origin$ there.
// Only the watchers on the battlefield are walked; the moved card's own
// triggers are not (its trigger zones name the zone it left, which this port
// does not walk).
func (g *Game) checkLeftGraveyardTriggers(controller PlayerController, card CardID, dest ZoneType) {
	var matches []Ability
	c := g.Card(card)
	for _, pid := range g.Players() {
		for _, watcher := range g.traitHosts(pid) {
			w := g.Card(watcher)
			if w.Def == nil {
				continue
			}
			for face := range w.triggerFaces {
				for _, t := range face.Triggers {
					if !strings.EqualFold(t.Name, "ChangesZone") || !hasZoneOrAny(t, "Origin", Graveyard) ||
						!hasZoneOrAny(t, "Destination", dest) || !changesZoneResolvable(t) {
						continue
					}
					validCard, ok := t.Param("ValidCard")
					if !ok || !Matches(g, c, valid.Parse(validCard), w.Controller(), watcher) {
						continue
					}
					if sub, api, optional, ok := triggerEffectAPI(g, w, face.Amounts, t); ok {
						matches = append(matches, Ability{API: api, Source: watcher, Controller: w.Controller(), Params: sub, Amounts: face.Amounts, Optional: optional, triggered: face.objects(triggeredObjects{card: card}), staticTrigger: isStaticTrigger(t)})
					}
				}
			}
		}
	}
	g.pushTriggeredAbilities(controller, matches)
}

// otherLeftToTriggerMatches is checkLeftToTriggers' wider half: every
// permanent still on the battlefield gets its own Triggers walked against
// left. No entered == left skip is needed: left is already in dest.
func (g *Game) otherLeftToTriggerMatches(left CardID, dest ZoneType) []Ability {
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
					if !isLeftToTrigger(t, dest) {
						continue
					}
					validCard, ok := t.Param("ValidCard")
					if !ok {
						continue
					}
					if !Matches(g, leaving, valid.Parse(validCard), w.Controller(), watcher) {
						continue
					}
					if sub, api, optional, ok := triggerEffectAPI(g, w, face.Amounts, t); ok {
						matches = append(matches, Ability{API: api, Source: watcher, Controller: w.Controller(), Params: sub, Amounts: face.Amounts, Optional: optional, triggered: face.objects(triggeredObjects{card: left}), staticTrigger: isStaticTrigger(t)})
					}
				}
			}
		}
	}
	return matches
}
