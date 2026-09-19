// Replacement effects: CR 614, trimmed to the corpus's single largest real
// shape -- a permanent replacing its own "enters the battlefield" event with
// "enters the battlefield tapped" (CR 614.1, ReplaceMoved.java). 969 of the
// corpus's 2,210 real R:Event$ lines name Moved; 618 of those carry
// ReplaceWith$ pointing at a bare `DB$ Tap` -- Java's own ETBTapped/
// LandTapped-named SVar convention -- and are the only shape this file
// resolves. Every other Event$ value (DamageDone, Untap, Counter, Draw, ...)
// and every other Moved shape (Exile, a conditional tap, a chained
// SubAbility$) is a gap game-state.md's own trigger-firing-style account
// names, not a reason to have skipped the one shape that does resolve.
//
// Ported from
// forge-game/src/main/java/forge/game/replacement/{ReplacementHandler,ReplaceMoved,ReplacementEffect}.java,
// trimmed the same way trigger.go's own checkETBTriggers is: CR 616's own
// "more than one replacement effect could apply, the affected player
// chooses" procedure needs a PlayerController hook this port does not have,
// so it is not built at all -- moot for the one outcome (Tapped = true)
// every resolvable line here produces, since applying it more than once is
// a no-op, not a wrong answer.

package engine

import (
	"strings"

	"github.com/jczastkiewicz/crucible/internal/carddb/compile"
	"github.com/jczastkiewicz/crucible/internal/valid"
)

// checkMovedReplacement is CR 614.1's own "look at the event before it
// happens" for a card that just moved onto the battlefield -- called from
// every real "moves onto the battlefield" site this port has
// (permanentEffect.Resolve/attachEffect.Resolve -- castspell.go;
// Game.PlayLand -- land.go), right after Game.Move and, like
// checkETBTriggers, not from Game.Move itself (Matches depends on game.go;
// game.go cannot depend back on anything that calls it, enginelint's own
// acyclic-parts rule). It runs BEFORE checkETBTriggers: a replacement
// changes the event itself, so an "enters tapped" permanent must already be
// tapped by the time a "when this enters" trigger looks at it, the same
// ordering CR 614.1 gives every replacement over CR 603's own triggers.
//
// Checked against two sets of Replacements, the identical split
// checkETBTriggers/otherETBTriggerMatches (above) already established:
// moved's own ("CARDNAME enters tapped," ValidCard$ Card.Self, 587 of 618
// real lines this resolves) and every OTHER permanent already on the
// battlefield ("creatures your opponents control enter tapped," 31 of 618).
// Unlike a trigger match, a replacement match here needs no APNAP ordering
// and no separate collect-then-push step: the one outcome this file
// produces, Tapped = true, is idempotent, so the first real match found (in
// either loop) is applied directly and the search stops -- CR 616's own
// "which one applies" choice has no observable answer to get wrong when
// every candidate would produce the identical result.
func (g *Game) checkMovedReplacement(moved CardID, origin ZoneType) {
	movedCard := g.Card(moved)
	if movedCard.Def != nil {
		for _, face := range movedCard.Def.Faces {
			for _, r := range face.Replacements {
				if replacementTapsOnMove(g, r, movedCard, origin, movedCard.Controller(), moved) {
					movedCard.Tapped = true
					return
				}
			}
		}
	}
	for _, pid := range g.Players() {
		for _, watcher := range g.Zone(Battlefield, pid).Cards() {
			if watcher == moved {
				continue
			}
			w := g.Card(watcher)
			if w.Def == nil {
				continue
			}
			for _, face := range w.Def.Faces {
				for _, r := range face.Replacements {
					if replacementTapsOnMove(g, r, movedCard, origin, w.Controller(), watcher) {
						movedCard.Tapped = true
						return
					}
				}
			}
		}
	}
}

// replacementTapsOnMove reports whether r is a resolvable "enters tapped"
// replacement matching moved's own zone change: Event$ Moved, an optional
// Origin$/Destination$ restriction (present on 2 and 624 of the real
// ETBTapped-named lines respectively -- absence of either means
// unrestricted, ReplaceMoved.java's own hasParam guard), ValidCard$ matched
// against moved the identical way a trigger's own ValidCard$ is (Matches,
// valid.go), and a ReplaceWith$ sub-ability tapAbilityIsPlainTap (below)
// recognizes.
func replacementTapsOnMove(g *Game, r *compile.Ability, movedCard *Card, origin ZoneType, hostController PlayerID, host CardID) bool {
	if !strings.EqualFold(r.Name, "Moved") {
		return false
	}
	if !replacementZoneMatches(r, "Destination", Battlefield) {
		return false
	}
	if !replacementZoneMatches(r, "Origin", origin) {
		return false
	}
	validCard, ok := r.Param("ValidCard")
	if !ok {
		return false
	}
	if !Matches(g, movedCard, valid.Parse(validCard), hostController, host) {
		return false
	}
	for _, sub := range r.Subs {
		if strings.EqualFold(sub.Key, "ReplaceWith") {
			return tapAbilityIsPlainTap(sub.Ability)
		}
	}
	return false
}

// replacementZoneMatches reports whether r's key names zone among its
// comma-separated zone list, or carries no such param at all -- an absent
// Origin$/Destination$ is not a restriction (ReplaceMoved.java's own
// `hasParam` guard around each check), unlike trigger.go's own hasZone,
// which a trigger's own isETBTrigger/isDiesTrigger always require present.
func replacementZoneMatches(r *compile.Ability, key string, zone ZoneType) bool {
	v, ok := r.Param(key)
	if !ok {
		return true
	}
	for _, z := range strings.Split(v, ",") {
		if z == zone.String() {
			return true
		}
	}
	return false
}

// tapAbilityIsPlainTap reports whether a is the one ReplaceWith$ shape this
// port resolves: a bare `DB$ Tap` naming Defined$ Self or Defined$
// ReplacedCard -- Java's own distinction between "the card carrying this
// replacement" and "the card the replacement is actually about," identical
// here since this file only ever reaches a's own host through the card
// that is moving (never through a separate targeted/remembered reference) --
// and nothing else. ETB$ True, present on every real line, is read as part
// of a's own params but never checked: it exists in Java to mark the tap as
// happening as part of entering rather than a later, ordinary tap (relevant
// to a first-strike-of-untap-step check no card in this shape needs), not to
// gate whether the tap itself happens. Any other param -- SubAbility$ (5 of
// 624 real ETBTapped lines, a chained counter grant), ConditionPresent$/
// ConditionCheckSVar$ (LandTapped's own "unless" shapes, a checkland/
// slowland) -- skips the whole line rather than tapping unconditionally and
// guessing wrong (PORT-8/GO-7): applying half of "enters tapped unless you
// control a Mountain" is not a partial answer, it is the wrong one.
func tapAbilityIsPlainTap(a *compile.Ability) bool {
	if !strings.EqualFold(a.Name, "Tap") {
		return false
	}
	defined, ok := a.Param("Defined")
	if !ok || (!strings.EqualFold(defined, "Self") && !strings.EqualFold(defined, "ReplacedCard")) {
		return false
	}
	for _, p := range a.Params {
		switch strings.ToLower(p.Key) {
		case "db", "defined", "etb":
		default:
			return false
		}
	}
	return true
}
