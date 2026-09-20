// Replacement effects: CR 614. The corpus's own 1,711 real R:Event$ lines
// split across three shapes this file resolves and everything else:
//
//   - Moved (969 lines) -- a permanent replacing its own "enters the
//     battlefield" event with "enters the battlefield tapped" (CR 614.1,
//     ReplaceMoved.java). 618 of those carry ReplaceWith$ pointing at a bare
//     `DB$ Tap` -- Java's own ETBTapped/LandTapped-named SVar convention --
//     and are the only Moved shape this file resolves (checkMovedReplacement,
//     below).
//   - Untap (158 lines) -- CR 502.3/614.17's own "doesn't untap during its
//     controller's untap step" (untapBlocked, below): 156 name
//     `Layer$ CantHappen`, the shape ReplaceUntap.canReplace ports directly;
//     the other 2 name ReplaceWith$ instead, a genuine substitution this file
//     does not resolve.
//   - DamageDone (218 lines) -- CR 614's own "prevent all of this damage"
//     (damagePrevented/damagePreventedPlayer, below): 72 name `Prevent$ True`,
//     ReplacementHandler's own unconditional-void dispatch for that value
//     (ReplaceDamage.canReplace plus the handler's own Prevent$ branch); the
//     other 146 name ReplaceWith$ -- a real sub-ability substitution
//     (DB$ ReplaceEffect/ReplaceDamage/RemoveCounter/PutCounter/..., no single
//     shape anywhere near Moved's own 618-line concentration), not resolved.
//
// Every other Event$ value (Counter, Draw, GainLife, ...) is a gap
// game-state.md's own trigger-firing-style account names, not a reason to
// have skipped the shapes that do resolve.
//
// Ported from
// forge-game/src/main/java/forge/game/replacement/{ReplacementHandler,ReplaceMoved,ReplaceUntap,ReplaceDamage,ReplacementEffect}.java,
// trimmed the same way trigger.go's own checkETBTriggers is: CR 616's own
// "more than one replacement effect could apply, the affected player
// chooses" procedure needs a PlayerController hook this port does not have,
// so it is not built at all -- moot for every outcome this file produces
// (Tapped = true, blocked = true, prevented = true), since applying any of
// them more than once is a no-op, not a wrong answer: the first real match
// found is applied (or, for Untap/DamageDone, simply reported) directly and
// the search stops.

package engine

import (
	"strings"

	"github.com/jczastkiewicz/crucible/internal/carddb/compile"
	"github.com/jczastkiewicz/crucible/internal/expr"
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
				if shouldTap, matched := replacementTapsOnMove(g, r, movedCard, origin, movedCard.Controller(), moved, face.Amounts); matched {
					movedCard.Tapped = shouldTap
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
					if shouldTap, matched := replacementTapsOnMove(g, r, movedCard, origin, w.Controller(), watcher, face.Amounts); matched {
						movedCard.Tapped = shouldTap
						return
					}
				}
			}
		}
	}
}

// replacementRequirementsCheck ports ReplacementEffect.requirementsCheck --
// a general gate every replacement carries regardless of what Event$ it
// names, checked before its own shape-specific canReplace, mirroring
// triggerPhasesCheck's own role for triggers (trigger.go): the two are
// genuinely parallel Java methods (Trigger.phasesCheck /
// ReplacementEffect.requirementsCheck), not the same one reused, since
// Trigger and ReplacementEffect are sibling subclasses of TriggerReplacementBase
// rather than one inheriting from the other. Every consumer in this file
// (damagePreventionMatches, untapReplacementMatches, replacementTapsOnMove,
// below) had its own separate, narrower allow-list of extra params it
// tolerated before this landed; each now includes this call and widens its
// own allow-list to admit the keys this function itself reads (below),
// closing 7 of 10 previously-skipped real DamageDone|Prevent$ lines and 5 of
// 7 previously-skipped Untap|CantHappen lines for free, plus fixing a real,
// if narrow, wrong-firing bug on `replacementTapsOnMove`'s own side:
// archelos_lagoon_mystic.txt's own real "enters tapped" toggle names
// IsPresent$ Card.Self+tapped/+untapped restricting Archelos's own two
// replacement lines to only apply while ARCHELOS ITSELF is tapped/untapped
// respectively -- unchecked before this, both lines were reachable
// regardless of Archelos's own state (a genuine wrong answer, not a gap,
// since replacementTapsOnMove carried no allow-list at all to skip on
// instead of guessing).
//
// PlayerTurn$ (8 real R: lines combined across DamageDone/Draw/CreateToken/
// LifeReduced/TurnFaceUp, every one the literal value "True" -- 0 real lines
// use the Defined$-reference else-branch Java's own requirementsCheck also
// has, so only the literal-True branch is ported) checks
// game.getPhaseHandler().isPlayerTurn(hostController) directly.
// ActivePhases$ (1 real line, island_sanctuary.txt's own Draw shape) reuses
// phaseTriggerMatches (trigger.go, Mode$ Phase's own dispatch function) at
// its own key rather than Phase$'s, the identical general-purpose phase-list
// parser either way. triggerCommonRequirementsMet (trigger.go,
// CardTraitBase.meetsCommonRequirements's own port) is then called outright
// -- ReplacementEffect.requirementsCheck's own final line calls the
// identical Java method a Trigger's own performTest already does, so this
// port's identical shared function serves both for the same reason.
func replacementRequirementsCheck(g *Game, host *Card, amounts map[string]expr.Amount, r *compile.Ability) bool {
	if v, ok := r.Param("PlayerTurn"); ok {
		if !strings.EqualFold(v, "True") {
			// Java's own else-branch (a Defined$ player reference rather than
			// the literal "True") -- 0 real lines use it, so skipping rather
			// than resolving it is the honest answer, not a guess (GO-7).
			return false
		}
		if g.ActivePlayer() != host.Controller() {
			return false
		}
	}
	if _, ok := r.Param("ActivePhases"); ok {
		if !phaseTriggerMatches(r, "ActivePhases", g.ActivePhase()) {
			return false
		}
	}
	return triggerCommonRequirementsMet(g, host, amounts, r)
}

// replacementTapsOnMove reports whether r is a resolvable "enters tapped"
// replacement matching moved's own zone change (matched, the second return
// value) and, if so, whether it actually taps (shouldTap, the first): Event$
// Moved, an optional Origin$/Destination$ restriction (present on 2 and 624
// of the real ETBTapped-named lines respectively -- absence of either means
// unrestricted, ReplaceMoved.java's own hasParam guard), ValidCard$ matched
// against moved the identical way a trigger's own ValidCard$ is (Matches,
// valid.go), replacementRequirementsCheck (above), and a ReplaceWith$
// sub-ability tapAbilityResolvesTap (below) recognizes -- matched true
// whether or not the checkland-style condition inside that sub-ability
// actually holds, since CR 616's own "which replacement applies" choice is
// decided by ReplaceWith$ naming a resolvable shape at all, not by what that
// shape's own resolution produces.
func replacementTapsOnMove(g *Game, r *compile.Ability, movedCard *Card, origin ZoneType, hostController PlayerID, host CardID, amounts map[string]expr.Amount) (shouldTap, matched bool) {
	if !strings.EqualFold(r.Name, "Moved") {
		return false, false
	}
	if !replacementZoneMatches(r, "Destination", Battlefield) {
		return false, false
	}
	if !replacementZoneMatches(r, "Origin", origin) {
		return false, false
	}
	validCard, ok := r.Param("ValidCard")
	if !ok {
		return false, false
	}
	if !Matches(g, movedCard, valid.Parse(validCard), hostController, host) {
		return false, false
	}
	if !replacementRequirementsCheck(g, g.Card(host), amounts, r) {
		return false, false
	}
	for _, sub := range r.Subs {
		if strings.EqualFold(sub.Key, "ReplaceWith") {
			return tapAbilityResolvesTap(g, sub.Ability, g.Card(host), amounts)
		}
	}
	return false, false
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

// tapAbilityResolvesTap reports whether a is a resolvable ReplaceWith$ shape
// (recognized, the second return value) and, if so, whether it actually taps
// (shouldTap, the first): a bare `DB$ Tap` naming Defined$ Self or Defined$
// ReplacedCard -- Java's own distinction between "the card carrying this
// replacement" and "the card the replacement is actually about," identical
// here since this file only ever reaches a's own host through the card that
// is moving (never through a separate targeted/remembered reference) --
// optionally gated by SpellAbilityCondition's own ConditionPresent$/
// ConditionCompare$/ConditionCheckSVar$/ConditionSVarCompare$
// (subAbilityConditionMet, condition.go) and nothing else. ETB$ True,
// present on every real line, is read as part of a's own params but never
// checked: it exists in Java to mark the tap as happening as part of
// entering rather than a later, ordinary tap (relevant to a
// first-strike-of-untap-step check no card in this shape needs), not to gate
// whether the tap itself happens. 618 of the corpus's 624 real ETBTapped
// lines carry no Condition-family param at all (shouldTap always true once
// recognized); 140 more real DB$ Tap lines do -- LandTapped's own checkland/
// slowland "unless" shape (Rootbound Crag: "enters tapped unless you control
// a Mountain or a Forest") -- 116 of those resolving through
// subAbilityConditionMet, the rest (SubAbility$, ConditionDefined$,
// ConditionPlayerTurn$, ConditionPhases$) still recognized false: applying
// half of "enters tapped unless you control a Mountain" would be a wrong
// answer, not a partial one, so a's own unresolved-param guard
// (subAbilityUnresolvedParams, condition.go) refuses the whole ability
// rather than tapping unconditionally and guessing wrong (PORT-8/GO-7). Any
// param past db/defined/etb/the four Condition keys, this function's own
// separate switch -- skips (recognized false) for the identical reason.
// replacementActiveZones parses r's own ActiveZones$ -- the zone(s) its host
// itself must occupy for r to apply at all (ReplacementEffect's own
// zonesCheck, distinct from a trigger's TriggerZones$, though both are the
// identical comma-list-of-zone-names shape). Absent is Battlefield alone,
// the corpus's own overwhelming default for both families below (105 of 156
// real Untap|CantHappen lines name it explicitly; 41 of 72 real
// DamageDone|Prevent lines do); an unrecognized zone name skips the whole
// line rather than guessing (GO-7), the identical contract validCountZones
// (amount.go) already has for a Count$Valid<Zone> suffix.
func replacementActiveZones(r *compile.Ability) ([]ZoneType, bool) {
	v, ok := r.Param("ActiveZones")
	if !ok {
		return []ZoneType{Battlefield}, true
	}
	zones := make([]ZoneType, 0, 1)
	for _, name := range strings.Split(v, ",") {
		z, ok := ZoneByName(name)
		if !ok {
			return nil, false
		}
		zones = append(zones, z)
	}
	return zones, true
}

// hostInActiveZones reports whether hostZone is one of r's own ActiveZones$.
func hostInActiveZones(r *compile.Ability, hostZone ZoneType) bool {
	zones, ok := replacementActiveZones(r)
	if !ok {
		return false
	}
	for _, z := range zones {
		if z == hostZone {
			return true
		}
	}
	return false
}

// replacementZones is every zone a replacement's own host is worth checking
// in, across every player: Battlefield alone would miss the 2 real
// Command-zone lines each of Untap|CantHappen and DamageDone|Prevent carries
// (an emblem- or effect-shaped host, not a permanent), so both callers below
// walk this pair rather than Battlefield alone.
var replacementZones = [...]ZoneType{Battlefield, Command}

// untapBlocked is CR 502.3/614.17's own "can't happen" replacement applied
// to CR 502's own untap step -- Card.canUntap's own cantHappenCheck,
// ReplaceUntap.canReplace ported directly. Walks every player's own
// Battlefield/Command looking for a replacement whose ValidCard$ matches
// card -- the first match found blocks the untap outright, CR 616's own
// "more than one could apply" choice producing the identical outcome
// (blocked) no matter which is picked, the same reasoning this file's own
// doc comment already gives for checkMovedReplacement.
//
// ValidStepTurnToController$ (154 of 156 real lines, always "You") is not
// checked: the one caller, untapStep (turn.go), only ever considers cards
// g.activePlayer already controls, so "the untapping player is this card's
// own controller" already holds by construction for every real value this
// param carries -- Java's own Untap.doUntap loop has the identical
// invariant for its own "self" untap pass (the only one this port models;
// doUntap's own "untap a card you don't control" branch,
// StaticAbilityUntapOtherPlayer, is not built, no card grants that
// permission yet).
func (g *Game) untapBlocked(card *Card) bool {
	for _, pid := range g.Players() {
		for _, z := range replacementZones {
			for _, host := range g.Zone(z, pid).Cards() {
				h := g.Card(host)
				if h.Def == nil {
					continue
				}
				for _, face := range h.Def.Faces {
					for _, r := range face.Replacements {
						if untapReplacementMatches(g, r, card, h.Controller(), host, z, face.Amounts) {
							return true
						}
					}
				}
			}
		}
	}
	return false
}

// untapReplacementMatches is ReplaceUntap.canReplace's own resolvable half:
// Event$ Untap, Layer$ CantHappen (156 of 158 real lines; the other 2 name
// ReplaceWith$ instead -- a genuine substitution, not a "can't happen," a
// different shape this file does not resolve), r's own host in one of its
// own ActiveZones$ (replacementActiveZones/hostInActiveZones, above),
// ValidCard$ matched against card the identical way checkMovedReplacement's
// own ValidCard$ already is, and replacementRequirementsCheck (above), which
// now folds in IsPresent$/CheckSVar$/SVarCompare$ generically -- closing 5 of
// the 7 real lines this shape used to skip for naming one of those three.
// Any param besides the ones real corpus lines pair with this shape
// (Secondary$, purely descriptive, plus whatever replacementRequirementsCheck
// itself reads) still skips the whole line rather than guessing (GO-7):
// EnduringStory$/AddSVar$ carry the remaining 2 of 156, each its own further
// restriction this file cannot evaluate.
func untapReplacementMatches(g *Game, r *compile.Ability, card *Card, hostController PlayerID, host CardID, hostZone ZoneType, amounts map[string]expr.Amount) bool {
	if !strings.EqualFold(r.Name, "Untap") {
		return false
	}
	layer, ok := r.Param("Layer")
	if !ok || !strings.EqualFold(layer, "CantHappen") {
		return false
	}
	for _, p := range r.Params {
		switch strings.ToLower(p.Key) {
		case "event", "layer", "description", "validcard", "validstepturntocontroller", "activezones", "secondary",
			"ispresent", "checksvar", "svarcompare":
		default:
			return false
		}
	}
	if !hostInActiveZones(r, hostZone) {
		return false
	}
	validCard, ok := r.Param("ValidCard")
	if !ok {
		return false
	}
	if !Matches(g, card, valid.Parse(validCard), hostController, host) {
		return false
	}
	return replacementRequirementsCheck(g, g.Card(host), amounts, r)
}

// damagePrevented is CR 614's own "prevent all of this damage" shape
// (Prevent$ True) applied to a *Card target -- ReplaceDamage.canReplace's
// own resolvable half plus ReplacementHandler's own Prevent$ True dispatch
// (ReplacementResult.Prevented: nothing replaces the event, it simply does
// not happen). The two real damage-dealing call sites this port has
// (dealPermanentDamage/dealPlayerDamage, combatdamage.go) check this before
// marking any damage or emitting DamageDealt -- a prevented damage instance
// never happened, the same "look at the event before it happens" ordering
// this file's own doc comment already gives CR 614 over CR 603's own
// triggers.
func (g *Game) damagePrevented(source, target CardID, isCombat bool) bool {
	targetCard := g.Card(target)
	for _, pid := range g.Players() {
		for _, z := range replacementZones {
			for _, host := range g.Zone(z, pid).Cards() {
				h := g.Card(host)
				if h.Def == nil {
					continue
				}
				for _, face := range h.Def.Faces {
					for _, r := range face.Replacements {
						if !damagePreventionMatches(g, r, source, h.Controller(), host, z, isCombat, face.Amounts) {
							continue
						}
						if validTarget, ok := r.Param("ValidTarget"); ok &&
							!Matches(g, targetCard, valid.Parse(validTarget), h.Controller(), host) {
							continue
						}
						return true
					}
				}
			}
		}
	}
	return false
}

// damagePreventedPlayer is damagePrevented's own player-target twin, ported
// for the identical reason checkDamageDoneTriggersToPlayer (trigger.go) is
// checkDamageDoneTriggersToCard's own: ValidTarget$ matched against a Player
// through matchesPlayerSpec rather than Matches.
func (g *Game) damagePreventedPlayer(source CardID, target PlayerID, isCombat bool) bool {
	for _, pid := range g.Players() {
		for _, z := range replacementZones {
			for _, host := range g.Zone(z, pid).Cards() {
				h := g.Card(host)
				if h.Def == nil {
					continue
				}
				for _, face := range h.Def.Faces {
					for _, r := range face.Replacements {
						if !damagePreventionMatches(g, r, source, h.Controller(), host, z, isCombat, face.Amounts) {
							continue
						}
						if validTarget, ok := r.Param("ValidTarget"); ok {
							matched, recognized := matchesPlayerSpec(g, target, h.Controller(), validTarget)
							if !recognized || !matched {
								continue
							}
						}
						return true
					}
				}
			}
		}
	}
	return false
}

// damagePreventionMatches is damagePrevented/damagePreventedPlayer's own
// shared half: Event$ DamageDone, Prevent$ True, r's own host in one of its
// own ActiveZones$, ValidSource$ matched against source the identical way
// damageDoneMatches' own ValidSource$ already is (trigger.go), IsCombat$
// agreeing with isCombat, and replacementRequirementsCheck (above), which
// now folds in PlayerTurn$/CheckSVar$/SVarCompare$/IsPresent$ generically --
// closing 7 of the 10 real lines this shape used to skip for naming one of
// those four (guardian_naga_banishing_coils.txt's own real "can't be dealt
// damage during your turn," PlayerTurn$ True, among them). Any param besides
// the ones real corpus lines pair with this shape (Secondary$, purely
// descriptive, plus whatever replacementRequirementsCheck itself reads)
// still skips the whole line: ValidCause$/RelativeToSource$/DamageAmount$/
// CauseIsSource$ together carry the remaining 3 of 72 real lines this shape
// would otherwise match, each its own further restriction this file cannot
// evaluate (GO-7).
func damagePreventionMatches(g *Game, r *compile.Ability, source CardID, hostController PlayerID, host CardID, hostZone ZoneType, isCombat bool, amounts map[string]expr.Amount) bool {
	if !strings.EqualFold(r.Name, "DamageDone") {
		return false
	}
	prevent, ok := r.Param("Prevent")
	if !ok || !strings.EqualFold(prevent, "True") {
		return false
	}
	for _, p := range r.Params {
		switch strings.ToLower(p.Key) {
		case "event", "prevent", "description", "validtarget", "activezones", "validsource", "iscombat", "secondary",
			"playerturn", "checksvar", "svarcompare", "ispresent":
		default:
			return false
		}
	}
	if !hostInActiveZones(r, hostZone) {
		return false
	}
	if validSource, ok := r.Param("ValidSource"); ok && !Matches(g, g.Card(source), valid.Parse(validSource), hostController, host) {
		return false
	}
	if combat, ok := r.Param("IsCombat"); ok && strings.EqualFold(combat, "True") != isCombat {
		return false
	}
	return replacementRequirementsCheck(g, g.Card(host), amounts, r)
}

// drawPrevented is CR 121.4/614's own "prevent this draw" shape (Prevent$
// True) applied to CR 120.3's own "draw a card" event -- ReplaceDraw's own
// resolvable half plus ReplacementHandler's own Prevent$ True dispatch, the
// identical contract damagePreventedPlayer already has for a different
// Event$. DrawCards (turn.go) checks this before drawing each individual
// card -- Java's own Player.doDraw runs the identical Event$ Draw
// replacement check before ever looking at whether the library is empty, so
// a prevented draw does not count as CR 704.5b's own "attempted to draw from
// an empty library" either: this port's own DrawCards checks drawPrevented
// first and, when it reports true, never reaches the empty-library check at
// all for that card -- possessed_portal.txt's own real "if a player would
// draw a card, that player skips that draw instead" would otherwise still
// lose a player to state-based action 704.5b even though the draw it never
// got to attempt was the one thing keeping the library from mattering.
//
// 2 of the corpus's own 39 real Event$ Draw lines naming Prevent$ True
// resolve end to end (possessed_portal.txt's own bare form, ValidPlayer$
// Player with no further restriction; living_conundrum.txt's own
// IsPresent$ Card.YouOwn | PresentZone$ Library | PresentCompare$ EQ0,
// "if you would draw while your library has no cards," resolved through
// replacementRequirementsCheck's own triggerCommonRequirementsMet fold-in
// with no code of its own needed). Not resolved: Optional$ (1 of 3 real
// Prevent$ lines) -- an interactive "may" confirm this port's own
// PlayerController has no hook for, the identical gap Discard's own
// Optional$/BecomesTarget's own OptionalDecider$ already document. The
// other 36 real Draw lines name ReplaceWith$ instead of Prevent$ -- a real
// substitution (DrawTwo/Dig/ExileTop/...), no single shape anywhere near
// Moved's own 618-line concentration, not resolved.
func (g *Game) drawPrevented(player PlayerID) bool {
	for _, pid := range g.Players() {
		for _, z := range replacementZones {
			for _, host := range g.Zone(z, pid).Cards() {
				h := g.Card(host)
				if h.Def == nil {
					continue
				}
				for _, face := range h.Def.Faces {
					for _, r := range face.Replacements {
						if !drawPreventionMatches(g, r, host, z, face.Amounts) {
							continue
						}
						if validPlayer, ok := r.Param("ValidPlayer"); ok {
							matched, recognized := matchesPlayerSpec(g, player, h.Controller(), validPlayer)
							if !recognized || !matched {
								continue
							}
						}
						return true
					}
				}
			}
		}
	}
	return false
}

// drawPreventionMatches is drawPrevented's own shared half: Event$ Draw,
// Prevent$ True, r's own host in one of its own ActiveZones$, and
// replacementRequirementsCheck (above). Any param besides the ones real
// corpus lines pair with this shape skips the whole line rather than
// guessing (GO-7): Optional$/NotFirstCardInDrawStep$/
// FirstExtraCardDrawnThisTurn$/ValidCause$ together carry the 1 of 3 real
// Prevent$ Draw lines this shape would otherwise match but cannot resolve.
func drawPreventionMatches(g *Game, r *compile.Ability, host CardID, hostZone ZoneType, amounts map[string]expr.Amount) bool {
	if !strings.EqualFold(r.Name, "Draw") {
		return false
	}
	prevent, ok := r.Param("Prevent")
	if !ok || !strings.EqualFold(prevent, "True") {
		return false
	}
	for _, p := range r.Params {
		switch strings.ToLower(p.Key) {
		case "event", "prevent", "description", "validplayer", "activezones", "secondary",
			"playerturn", "activephases", "checksvar", "svarcompare",
			"ispresent", "presentcompare", "presentzone", "presentplayer", "presentdefined":
		default:
			return false
		}
	}
	return hostInActiveZones(r, hostZone) && replacementRequirementsCheck(g, g.Card(host), amounts, r)
}

// gainLifePrevented is CR 119/614's own "prevent this life gain" shape
// (Prevent$ True) applied to CR 119.3's own "gain life" event -- the
// identical contract drawPrevented has for a different Event$.
// gainLifeEffect (gainlifeeffect.go) checks this before applying each
// player's own LifeAmount$, per player named by Defined$/ValidTgts$ -- CR
// 119's own life-gain replacement family game-state.md's own "M6's fourth
// effect: GainLife" section already flagged as entirely unbuilt is real now
// for its one directly resolvable real shape.
//
// 1 of the corpus's own 21 real Event$ GainLife lines resolves end to end,
// and it is also the ONLY one naming Prevent$ at all: sulfuric_vortex.txt's
// own bare "if a player would gain life, that player gains no life instead"
// (Prevent$ True, no ValidPlayer$ at all -- every player's own life gain is
// prevented, not just the caster's). The other 20 real lines all name
// ReplaceWith$ instead -- GainDouble/RLoseLife/Draw among them, each
// referencing an SVar whose own LifeAmount$/NumCards$ needs "the amount of
// life that would have been gained" as a runtime X/Y value this port's
// resolveAmount has no way to read back (a real mechanism gap distinct from
// an unresolved param -- Java's own AbilityKey.ReplacedAmount threading, not
// built), not resolved.
func (g *Game) gainLifePrevented(player PlayerID) bool {
	for _, pid := range g.Players() {
		for _, z := range replacementZones {
			for _, host := range g.Zone(z, pid).Cards() {
				h := g.Card(host)
				if h.Def == nil {
					continue
				}
				for _, face := range h.Def.Faces {
					for _, r := range face.Replacements {
						if !gainLifePreventionMatches(g, r, host, z, face.Amounts) {
							continue
						}
						if validPlayer, ok := r.Param("ValidPlayer"); ok {
							matched, recognized := matchesPlayerSpec(g, player, h.Controller(), validPlayer)
							if !recognized || !matched {
								continue
							}
						}
						return true
					}
				}
			}
		}
	}
	return false
}

// gainLifePreventionMatches is gainLifePrevented's own shared half: Event$
// GainLife, Prevent$ True, r's own host in one of its own ActiveZones$, and
// replacementRequirementsCheck (above). No real corpus line combines
// Prevent$ True with any param outside this shape's own allow-list --
// sulfuric_vortex.txt's own line is the entire real GainLife|Prevent$
// population, so this allow-list is wider than the corpus strictly needs
// today, kept symmetric with drawPreventionMatches' own identical shape
// rather than pared down to one card's exact param set.
func gainLifePreventionMatches(g *Game, r *compile.Ability, host CardID, hostZone ZoneType, amounts map[string]expr.Amount) bool {
	if !strings.EqualFold(r.Name, "GainLife") {
		return false
	}
	prevent, ok := r.Param("Prevent")
	if !ok || !strings.EqualFold(prevent, "True") {
		return false
	}
	for _, p := range r.Params {
		switch strings.ToLower(p.Key) {
		case "event", "prevent", "description", "validplayer", "activezones", "secondary",
			"playerturn", "activephases", "checksvar", "svarcompare",
			"ispresent", "presentcompare", "presentzone", "presentplayer", "presentdefined":
		default:
			return false
		}
	}
	return hostInActiveZones(r, hostZone) && replacementRequirementsCheck(g, g.Card(host), amounts, r)
}

func tapAbilityResolvesTap(g *Game, a *compile.Ability, host *Card, amounts map[string]expr.Amount) (shouldTap, recognized bool) {
	if !strings.EqualFold(a.Name, "Tap") {
		return false, false
	}
	defined, ok := a.Param("Defined")
	if !ok || (!strings.EqualFold(defined, "Self") && !strings.EqualFold(defined, "ReplacedCard")) {
		return false, false
	}
	for _, p := range a.Params {
		switch strings.ToLower(p.Key) {
		case "db", "defined", "etb",
			"conditionpresent", "conditioncompare", "conditionchecksvar", "conditionsvarcompare":
		default:
			return false, false
		}
	}
	return subAbilityConditionMet(g, host, amounts, a), true
}
