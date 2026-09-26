// Choosing targets: CR 601.2c (a spell) / 603.3b (a triggered ability) --
// "choices... including targets... are made" the moment the ability is put
// on the stack. This port has exactly one place that happens today,
// pushTriggeredAbilities (trigger.go), so resolveTargets is called from
// there rather than from a separate step of its own; a future cast path for
// an Instant/Sorcery naming its own top-level ValidTgts$ (not built --
// CastSpell, castspell.go, only casts a permanent or an Aura today) would
// call it from wherever that lands too.

package engine

import (
	"github.com/jczastkiewicz/crucible/internal/valid"
)

// targetUnresolvedParams names ValidTgts$'s own structural siblings this
// port does not parse: Radiance$ (4 real corpus lines -- "and each other
// permanent that shares a color with it," a second, derived candidate set
// no single ValidTgts$ evaluation produces), TargetsForEachPlayer$/
// TargetsWithDefinedController$/TargetUnique$ (0 real lines each) -- every
// one its own further mechanic. A line naming any of these is treated the
// same as CR 603.3c's own "no legal targets" case (below) rather than
// erroring: both mean the ability does not do anything, and this port has
// no way to tell the difference from outside without building the shape
// (PORT-8/GO-7's "skip rather than guess," folded into the identical bucket
// a genuinely empty candidate set already uses since the two are
// observationally identical).
var targetUnresolvedParams = [...]string{
	"Radiance", "TargetsForEachPlayer", "TargetsWithDefinedController", "TargetUnique",
}

// resolveTargets is CR 601.2c/603.3b's own "choose targets," and reports
// whether a is still eligible to be pushed onto the stack at all. true with
// a.Targets left nil means there was nothing to target in the first place --
// no ValidTgts$ named. false means either CR 603.3c's own real rule ("if
// the ability requires a target and there are no legal targets, it doesn't
// go on the stack") or a target shape targetUnresolvedParams names above --
// the two are folded into the same outcome rather than given an error
// return, because a caller cannot act on "an ability the game decided not
// to put on the stack" any differently than "an ability this port cannot
// parse the targeting for": either way, nothing happens, and GO-7's own
// "fail one game, not the batch" reasoning does not apply to a card that
// was never going to do anything this port can tell.
//
// TargetMin$/TargetMax$ (1/1 when neither is named, TargetRestrictions.
// java's own getOrDefault) resolve through resolveNamedAmount exactly as
// every other numeric param already does. targetCandidates (below) always
// evaluates ValidTgts$ against both players and cards and unions whatever
// matches -- a real corpus spec regularly names both ("Any", CR 115's own
// "any target"; `Player,Planeswalker`, written out explicitly, 273 real
// lines corpus-wide), so there is no single "shape" to settle up front the
// way an earlier version of this function tried to.
func (g *Game) resolveTargets(controller PlayerController, a *Ability) bool {
	choice, named, ok := g.targetChoiceFor(a)
	if !named {
		return true
	}
	if !ok {
		return false
	}
	a.Targets = controller.ChooseTargets(g, a.Controller, choice.candidates, choice.min, choice.max)
	return true
}

// targetChoice is one targeting part's CR 601.2c question: the legal
// candidates and how many of them to choose.
type targetChoice struct {
	candidates []EntityID
	min, max   int
}

// targetChoiceFor builds a's targetChoice from its ValidTgts$/TargetType$/
// TargetMin$/TargetMax$, the scan resolveTargets asks the controller over
// and a copy's new targets are chosen from (CR 707.10c, copySpell,
// copyspellabilityeffect.go). named is false when a names no ValidTgts$ at
// all; ok is false when it does but has no legal candidate or a shape
// targetUnresolvedParams names -- resolveTargets' own two outcomes.
func (g *Game) targetChoiceFor(a *Ability) (choice targetChoice, named, ok bool) {
	validTgts, named := a.Params.Param("ValidTgts")
	if !named && a.API == APIEarthbend {
		// EarthbendEffect.buildSpellAbility sets the target restriction
		// itself; no script line names it.
		validTgts, named = "Land.YouCtrl", true
	}
	if !named {
		return targetChoice{}, false, true
	}
	for _, key := range targetUnresolvedParams {
		if _, ok := a.Params.Param(key); ok {
			return targetChoice{}, true, false
		}
	}

	source := g.Card(a.Source)
	minStr, ok := a.Params.Param("TargetMin")
	if !ok {
		minStr = "1"
	}
	maxStr, ok := a.Params.Param("TargetMax")
	if !ok {
		maxStr = "1"
	}
	targetMin, ok := resolveNamedAmount(g, a.Amounts, source, minStr)
	if !ok {
		return targetChoice{}, true, false
	}
	targetMax, ok := resolveNamedAmount(g, a.Amounts, source, maxStr)
	if !ok {
		return targetChoice{}, true, false
	}

	var candidates []EntityID
	if targetType, ok := a.Params.Param("TargetType"); ok {
		// TargetType$ Spell names spells on the stack (CR 115.1a); a spell
		// here is a card in the Stack zone. Activated/Triggered abilities
		// on the stack are not targetable objects in this port.
		if targetType != "Spell" {
			return targetChoice{}, true, false
		}
		candidates = g.stackSpellCandidates(a.Controller, a.Source, validTgts)
	} else if a.API == APICopySpellAbility {
		// CopySpellAbilityEffect.buildSpellAbility sets the target zone to
		// the stack whether or not TargetType$ is named
		// (CopySpellAbilityEffect.java:28-33): Mischievous Quanar's
		// ValidTgts$ Instant,Sorcery names spells, not battlefield cards.
		candidates = g.stackSpellCandidates(a.Controller, a.Source, validTgts)
	} else {
		candidates = g.targetCandidates(a.Controller, a.Source, validTgts)
	}
	if len(candidates) == 0 {
		return targetChoice{}, true, false
	}
	return targetChoice{candidates: candidates, min: targetMin, max: targetMax}, true, true
}

// targetCandidates is the union of spec evaluated against every player still
// in the game (matchesPlayerSpec, valid.go; a player who has lost is never a
// legal target, the identical exclusion definedPlayers's own
// `if (!p.isInGame())` reading already makes) AND against every card on any
// player's battlefield (Matches, valid.go) -- CR's own implicit "target
// creature" scope, and the only zone 0 real corpus TgtZone$ lines ever ask
// this port to look anywhere else than.
//
// Both pools are always tried, never one or the other picked by a spec's own
// shape: Java's own TargetRestrictions.getAllCandidates (CR 115's own "any
// target" candidate collection) does the identical thing, unconditionally
// probing game.getPlayers() and game.getCardsIn(zone) for every ValidTgts$
// spec -- an ordinary card-shaped spec ("Creature.YouCtrl") simply matches
// zero players the same way an ordinary player-shaped one ("Opponent")
// matches zero cards, filtering happening entirely inside matchesPlayerSpec/
// Matches rather than by picking a pool up front. This port tried the
// "pick one pool by a trial match" shortcut first; it is unsound for any
// spec whose comma-separated alternatives mix a player-shaped and a
// card-shaped one -- CR 115's own "Any" (matchesPlayerBase's own "Any" case)
// is the single-token example, but the corpus also writes it out explicitly
// (`Player,Planeswalker`, 273 real lines corpus-wide) -- so the union is not
// an "Any"-only special case, it is the general, correct shape.
func (g *Game) targetCandidates(controller PlayerID, source CardID, spec string) []EntityID {
	var candidates []EntityID
	for _, pid := range g.Players() {
		if g.Player(pid).Lost {
			continue
		}
		if matched, _ := matchesPlayerSpec(g, pid, controller, source, spec); matched {
			candidates = append(candidates, PlayerEntity(pid))
		}
	}

	parsed := valid.Parse(spec)
	for _, pid := range g.Players() {
		for _, id := range g.Zone(Battlefield, pid).Cards() {
			if Matches(g, g.Card(id), parsed, controller, source) {
				candidates = append(candidates, CardEntity(id))
			}
		}
	}
	return candidates
}

// stackSpellCandidates is every spell on the stack -- a card in the Stack
// zone, top first -- matching spec.
func (g *Game) stackSpellCandidates(controller PlayerID, source CardID, spec string) []EntityID {
	parsed := valid.Parse(spec)
	var candidates []EntityID
	for i := len(g.stack) - 1; i >= 0; i-- {
		id := g.stack[i].Source
		c := g.Card(id)
		if c.Zone != Stack || containsEntity(candidates, CardEntity(id)) {
			continue
		}
		if Matches(g, c, parsed, controller, source) {
			candidates = append(candidates, CardEntity(id))
		}
	}
	return candidates
}

// targetsStillLegal is CR 608.2b, checked by ResolveStack (ADR-0018) right
// before an ability would resolve -- narrowed to the one shape this port can
// re-check soundly: an Aura's own single cast-time Target (castAura,
// castspell.go). Every other ability keeps resolveTargets' own contract
// unrevisited: a's own doc comment already says a chosen Targets answer "is
// NOT re-checked... trust the controller's answer" (ability.go), the same
// stance every decision method in control.go documents for its own return
// value, and targetCandidates' own scan (below) is scoped for finding NEW
// candidates at push time, not for confirming an already-chosen one is still
// among them: TestRemoveFromGameSpellOnStack (pack3shapes_test.go) targets a
// spell still on the Stack zone through a plain ValidTgts$ Card line with no
// TargetType$ Spell at all -- legal at push time only because
// targetCandidates found some OTHER Battlefield card matching "Card" to
// satisfy resolveTargets' own nonempty-candidates gate, with the actually
// chosen target trusted separately. Recomputing that same scan here and
// intersecting it against a.Targets would wrongly fizzle a real, already
// passing case; a general re-check needs its own design, not a reuse of
// resolveTargets' own push-time helpers.
func (g *Game) targetsStillLegal(a *Ability) bool {
	if a.API != APIAttach || a.Target == NoCard {
		return true
	}
	return g.auraTargetStillLegal(a)
}

// auraTargetStillLegal is targetsStillLegal's own Aura branch: a's Target
// (castAura, castspell.go) is still legal only if it is still on the
// battlefield, still matches self's own Enchant restriction, and still does
// not refuse self outright (hostRefusesEnchant, staticability.go -- the
// identical two checks enchantTargets already ran to build the candidate set
// this target was chosen from, castspell.go).
func (g *Game) auraTargetStillLegal(a *Ability) bool {
	c := g.Card(a.Source)
	target := g.Card(a.Target)
	if target.Zone != Battlefield {
		return false
	}
	spec, ok := enchantSpec(c)
	if !ok {
		return true
	}
	return Matches(g, target, spec, a.Controller, a.Source) && !hostRefusesEnchant(g, c, a.Target)
}
