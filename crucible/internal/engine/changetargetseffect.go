package engine

//enginelint:allow ability card condition control defined effecthelpers game id stack targeting trigger valid

import (
	"fmt"

	"github.com/jczastkiewicz/crucible/internal/valid"
)

// changeTargetsEffect is ChangeTargetsEffect.java (CR 115.7): for each
// named spell still on the stack, Optional$ lets the activator stop first,
// then one of three shapes rewrites the targets of each of its targeting
// parts (the spell itself, then each Charm mode -- Java's sub-instance
// chain):
//
//   - ChangeSingleTarget$: the activator picks one (part, target) pair and
//     it becomes DefinedMagnet$'s card if that card is not already among the
//     part's targets and the part could target it (Spellskite, Mizzium
//     Meddler).
//   - DefinedMagnet$ alone: every part that could target DefinedMagnet$'s
//     card has its targets replaced by that one card (Muck Drubb).
//   - Otherwise: the activator chooses as many new targets as the part had,
//     from what the part could target, TargetRestriction$ narrowing it; with
//     fewer candidates than that the part keeps its targets
//     (PlayerControllerHuman.chooseNewTargetsFor, TargetSelection.
//     chooseTargets with numTargets = the old count).
//
// Each object newly targeted raises BecomesTarget (checkBecomesTargetTriggers)
// the way the spell's own cast did. BecomesTargetOnce has no trigger mode in
// this port, at cast time or here.
//
// The spells and abilities are the ability's own targets (ValidTgts$,
// TargetType$ read by stackAbilityCandidates, targeting.go; an ability as its
// stack item, ADR-0041), Defined$ Targeted (the same list, a
// sub-ability sharing its parent's targets) or Defined$ TriggeredSpellAbility
// (the spell a Mode$ SpellCast trigger recorded).
//
// Ported from forge-game/src/main/java/forge/game/ability/effects/ChangeTargetsEffect.java's
// resolve (ChangeTargetsEffect.java:48-210).
type changeTargetsEffect struct{}

// changeTargetsUnresolvedParams are rejected before anything changes
// (PORT-8, GO-7): the
// condition params subAbilityConditionMet reads as never met, silently:
// ConditionTargetValidTargeting$/ConditionTargetsSingleTarget$ (Meddle,
// Quicksilver Dragon) and ConditionPlayerDefined$/ConditionPlayerContains$
// (Emissary of Grudges).
// TargetsWithControllerProperty$ is a canTargetSpellAbility filter
// stackAbilityCandidates does not read.
var changeTargetsUnresolvedParams = [...]string{
	"ConditionPlayerDefined", "ConditionPlayerContains", "TargetsWithControllerProperty",
}

// retargeted is one spell's newly targeted objects, held until every named
// spell is rewritten: checkBecomesTargetTriggers pushes onto the stack, and
// a push can move the stack item a pointer into g.stack names.
type retargeted struct {
	targets    []EntityID
	controller PlayerID
	// id, source and spell name the rewritten stack item for Ward: a retarget
	// is a new "becomes the target" event for the item's own controller.
	id     StackItemID
	source CardID
	spell  bool
}

func (changeTargetsEffect) Resolve(g *Game, a *Ability, controller PlayerController) error {
	if err := rejectParams(a, "ChangeTargets", changeTargetsUnresolvedParams[:]...); err != nil {
		return err
	}
	source := g.Card(a.Source)
	if !subAbilityConditionMet(g, source, a.Amounts, a.Params) {
		return nil
	}
	ids, err := changeTargetsSpells(g, a)
	if err != nil {
		return err
	}
	single := hasParam(a, "ChangeSingleTarget")
	magnet := NoCard
	magnetSpec, hasMagnet := a.Params.Param("DefinedMagnet")
	if hasMagnet {
		cards, err := definedCards(source, magnetSpec, a.refs())
		if err != nil {
			return fmt.Errorf("engine: ChangeTargets: DefinedMagnet$: %w", err)
		}
		if len(cards) > 0 {
			magnet = cards[0]
		}
	} else if single {
		return fmt.Errorf("engine: ChangeTargets: ChangeSingleTarget$ without DefinedMagnet$ not resolvable yet")
	}

	// Chooser$ (ChangeTargetsEffect.java:52-56): the players who decide, the
	// activator by default; naming nobody ends the effect. The first one decides
	// (no corpus line names two).
	chooser := a.Controller
	if spec, ok := a.Params.Param("Chooser"); ok {
		players, err := definedPlayers(g, a.Controller, a.Source, spec, a.refs())
		if err != nil {
			return fmt.Errorf("engine: ChangeTargets: Chooser$: %w", err)
		}
		if len(players) == 0 {
			return nil
		}
		chooser = players[0]
	}
	var fired []retargeted
	for _, id := range ids {
		item, ok := g.stackItem(id)
		if !ok {
			// "If there isn't a Stack Instance, there isn't really a target."
			continue
		}
		if hasParam(a, "Optional") && !controller.ConfirmEffect(g, chooser, a.Source) {
			continue
		}
		parts, err := retargetParts(item)
		if err != nil {
			return err
		}
		var changed []EntityID
		stop := false
		switch {
		case single:
			changed, stop, err = g.changeSingleTarget(controller, a, item, parts, magnet)
		case hasMagnet:
			changed, err = g.changeToMagnet(item, parts, magnet)
		case hasParam(a, "RandomTarget"):
			changed, err = g.randomNewTargets(a, item, parts)
		default:
			changed, err = g.chooseNewTargets(controller, a, chooser, item, parts)
		}
		if err != nil {
			return err
		}
		// A new target is checked at resolution as the object it is now
		// (CR 608.2b); a kept one keeps the stamp it was chosen with.
		g.stampTargets(item)
		if stop {
			// ChangeTargetsEffect.java:88-90 returns from resolve outright
			// when the spell has no target at all, skipping every spell
			// after it (PORT-7: parity with that early return).
			break
		}
		if len(changed) > 0 {
			fired = append(fired, retargeted{targets: changed, controller: item.Controller, id: item.ID, source: item.Source, spell: item.spell})
		}
	}
	for _, f := range fired {
		// isSpellSource false, as castInstantOrSorcery passes it for the
		// same spell's own cast-time targets (castspell.go); Aura spells
		// are refused by retargetParts.
		matches := g.checkBecomesTargetTriggers(f.targets, false, f.controller)
		if f.spell {
			matches = append(matches, g.checkWardTriggers(f.targets, f.source, f.controller)...)
		} else {
			matches = append(matches, g.checkWardTriggersForAbility(f.targets, f.id, f.source, f.controller)...)
		}
		g.pushTriggeredAbilities(controller, matches)
	}
	return nil
}

// changeTargetsSpells is getTargetSpells(sa): the stack items a names.
func changeTargetsSpells(g *Game, a *Ability) ([]StackItemID, error) {
	if _, ok := a.Params.Param("ValidTgts"); !ok {
		defined, _ := a.Params.Param("Defined")
		switch defined {
		case "Targeted":
		case "Remembered":
			// AbilityUtils.java:1235-1241: each remembered card's spell abilities
			// (a spell on the stack is its card's one) -- a copy is remembered as its
			// card (CopySpellAbility RememberCopies$).
			var ids []StackItemID
			for _, e := range g.Card(a.Source).Memory.Remembered() {
				if card, ok := e.AsCard(); ok {
					if id, ok := g.spellItemOf(card); ok {
						ids = append(ids, id)
					}
				}
			}
			return ids, nil
		case "TriggeredSpellAbility":
			if a.triggered.spellAbility == NoStackItem {
				return nil, fmt.Errorf("engine: ChangeTargets: Defined$ TriggeredSpellAbility: no triggering spell recorded")
			}
			return []StackItemID{a.triggered.spellAbility}, nil
		default:
			// TriggeredSourceSA, Remembered, ValidStack, SourceFirstSpell:
			// getDefinedSpellAbilities shapes this port does not build.
			return nil, fmt.Errorf("engine: ChangeTargets: Defined$ %q not resolvable yet", defined)
		}
	}
	var ids []StackItemID
	for _, t := range a.Targets {
		if id, ok := t.AsStackItem(); ok {
			// An activated or triggered ability (ADR-0041): Willbender, Bolt
			// Bend, Spellskite, Reroute.
			ids = append(ids, id)
			continue
		}
		card, ok := t.AsCard()
		if !ok {
			continue
		}
		if id, ok := g.spellItemOf(card); ok {
			ids = append(ids, id)
		}
	}
	return ids, nil
}

// retargetParts is item's targeting parts in Java's sub-instance order: the
// item itself when it targets, then each Charm mode that does. Modes is
// copied first: Game.Clone shares the stack's Modes backing array, and the
// parts returned are written through.
func retargetParts(item *Ability) ([]*Ability, error) {
	if item.API == APIAttach && item.Target != NoCard {
		return nil, fmt.Errorf("engine: ChangeTargets: changing an Aura spell's target not resolvable yet")
	}
	if subChainTargets(item.Params) {
		return nil, fmt.Errorf("engine: ChangeTargets: changing targets of a spell whose SubAbility$ targets not resolvable yet")
	}
	var parts []*Ability
	if partUsesTargeting(item) {
		parts = append(parts, item)
	}
	if item.Modes != nil {
		item.Modes = append([]Ability(nil), item.Modes...)
	}
	for i := range item.Modes {
		m := &item.Modes[i]
		if subChainTargets(m.Params) {
			return nil, fmt.Errorf("engine: ChangeTargets: changing targets of a spell whose SubAbility$ targets not resolvable yet")
		}
		if partUsesTargeting(m) {
			parts = append(parts, m)
		}
	}
	for _, p := range parts {
		if _, ok := p.Params.Param("DividedAsYouChoose"); ok {
			return nil, fmt.Errorf("engine: ChangeTargets: changing a divided target not resolvable yet")
		}
	}
	return parts, nil
}

// partUsesTargeting is SpellAbility.usesTargeting for a part: it names
// ValidTgts$ (Earthbend's restriction is built in, targetChoiceFor).
func partUsesTargeting(p *Ability) bool {
	if p.Params == nil {
		return false
	}
	if _, ok := p.Params.Param("ValidTgts"); ok {
		return true
	}
	return p.API == APIEarthbend
}

// partCandidates is what part could target now (SpellAbility.canTarget,
// TargetRestrictions.getAllCandidates): targetChoiceFor's scan for the
// part's own controller and host, less the spell or ability being changed
// itself (it cannot target itself, canTargetSpellAbility's
// this.equals(topSA)).
func (g *Game) partCandidates(item, part *Ability) ([]EntityID, error) {
	choice, named, ok := g.targetChoiceFor(part)
	if choice.err != nil {
		return nil, choice.err
	}
	if !named || !ok {
		return nil, nil
	}
	self := stackTargetEntity(item)
	var out []EntityID
	for _, c := range choice.candidates {
		if c == self {
			continue
		}
		out = append(out, c)
	}
	return out, nil
}

// changeSingleTarget is ChangeTargetsEffect.java:76-106: one (part, target)
// pair is picked -- automatically when there is only one, as
// PlayerControllerHuman.chooseTarget does -- and replaced by magnet (removed,
// then magnet appended, TargetChoices' own order) if the part does not
// already target magnet and could. stop reports the spell had no target at
// all, which ends the whole resolution in Java.
//
// The pick goes through ChooseTargets over the distinct targets; a target
// shared by two parts would need the pair itself chosen, a decision
// PlayerController does not have, so that case is an error.
func (g *Game) changeSingleTarget(controller PlayerController, a *Ability, item *Ability, parts []*Ability, magnet CardID) ([]EntityID, bool, error) {
	var owners []*Ability
	var targets []EntityID
	for _, p := range parts {
		for _, t := range p.Targets {
			if containsEntity(targets, t) {
				return nil, false, fmt.Errorf("engine: ChangeTargets: ChangeSingleTarget$ over a target two parts share not resolvable yet")
			}
			owners = append(owners, p)
			targets = append(targets, t)
		}
	}
	if len(targets) == 0 {
		return nil, true, nil
	}
	pick := 0
	if len(targets) > 1 {
		chosen := controller.ChooseTargets(g, a.Controller, targets, 1, 1)
		if err := checkChoice(chosen, targets, 1, 1); err != nil {
			return nil, false, fmt.Errorf("engine: ChangeTargets: target to change: %w", err)
		}
		for i, t := range targets {
			if t == chosen[0] {
				pick = i
			}
		}
	}
	if magnet == NoCard {
		return nil, false, nil
	}
	part, old, next := owners[pick], targets[pick], CardEntity(magnet)
	if containsEntity(part.Targets, next) {
		return nil, false, nil
	}
	candidates, err := g.partCandidates(item, part)
	if err != nil {
		return nil, false, err
	}
	if !containsEntity(candidates, next) {
		return nil, false, nil
	}
	rewritten := make([]EntityID, 0, len(part.Targets))
	for _, t := range part.Targets {
		if t != old {
			rewritten = append(rewritten, t)
		}
	}
	rewritten = append(rewritten, next)
	part.Targets = rewritten
	return []EntityID{next}, false, nil
}

// changeToMagnet is ChangeTargetsEffect.java:148-161: every part that could
// target magnet targets it alone.
func (g *Game) changeToMagnet(item *Ability, parts []*Ability, magnet CardID) ([]EntityID, error) {
	if magnet == NoCard {
		return nil, nil
	}
	next := CardEntity(magnet)
	var changed []EntityID
	for _, p := range parts {
		candidates, err := g.partCandidates(item, p)
		if err != nil {
			return nil, err
		}
		if !containsEntity(candidates, next) {
			continue
		}
		old := p.Targets
		p.Targets = []EntityID{next}
		if !containsEntity(old, next) && !containsEntity(changed, next) {
			changed = append(changed, next)
		}
	}
	return changed, nil
}

// chooseNewTargets is ChangeTargetsEffect.java:162-175 with
// PlayerControllerHuman.chooseNewTargetsFor: for each part, the activator
// chooses exactly as many targets as it had from what it could target now,
// TargetRestriction$ narrowing the offer (GameObjectPredicates.restriction,
// its "Other" read against the part's first card target, else a's host). A
// part with no targets, or fewer candidates than it had, keeps its own.
// Keeping a target is allowed: nothing in Forge's selection forces a
// different one.
func (g *Game) chooseNewTargets(controller PlayerController, a *Ability, chooser PlayerID, item *Ability, parts []*Ability) ([]EntityID, error) {
	restriction, hasRestriction := a.Params.Param("TargetRestriction")
	var parsed valid.Spec
	if hasRestriction {
		parsed = valid.Parse(restriction)
	}
	var changed []EntityID
	for _, p := range parts {
		n := len(p.Targets)
		if n == 0 {
			continue
		}
		candidates, err := g.partCandidates(item, p)
		if err != nil {
			return nil, err
		}
		if hasRestriction {
			restrictSource := a.Source
			for _, t := range p.Targets {
				if id, ok := t.AsCard(); ok {
					restrictSource = id
					break
				}
			}
			var kept []EntityID
			for _, c := range candidates {
				if g.entityMatches(c, restriction, parsed, a.Controller, restrictSource) {
					kept = append(kept, c)
				}
			}
			candidates = kept
		}
		if len(candidates) < n {
			continue
		}
		chosen := controller.ChooseTargets(g, chooser, candidates, n, n)
		if err := checkChoice(chosen, candidates, n, n); err != nil {
			return nil, fmt.Errorf("engine: ChangeTargets: new targets: %w", err)
		}
		old := p.Targets
		p.Targets = append([]EntityID(nil), chosen...)
		for _, t := range p.Targets {
			if !containsEntity(old, t) && !containsEntity(changed, t) {
				changed = append(changed, t)
			}
		}
	}
	return changed, nil
}

// randomNewTargets is ChangeTargetsEffect.java:111-152 (RandomTarget$): each
// targeting part gets one random legal target from what it could target now,
// RandomTargetRestriction$ (read for the activator and the host) narrowing the
// offer. A part with nothing left to pick aborts the whole change; the picks
// are applied once every part has one. Aggregates.random draws nothing for a
// single candidate (Chef's Kiss).
func (g *Game) randomNewTargets(a *Ability, item *Ability, parts []*Ability) ([]EntityID, error) {
	restriction, hasRestriction := a.Params.Param("RandomTargetRestriction")
	var parsed valid.Spec
	if hasRestriction {
		parsed = valid.Parse(restriction)
	}
	picks := make([]EntityID, len(parts))
	for i, p := range parts {
		if len(p.Targets) == 0 {
			continue
		}
		candidates, err := g.partCandidates(item, p)
		if err != nil {
			return nil, err
		}
		if hasRestriction {
			var kept []EntityID
			for _, c := range candidates {
				if g.entityMatches(c, restriction, parsed, a.Controller, a.Source) {
					kept = append(kept, c)
				}
			}
			candidates = kept
		}
		if len(candidates) == 0 {
			return nil, nil
		}
		picks[i] = candidates[g.randomIndex(len(candidates))]
	}
	var changed []EntityID
	for i, p := range parts {
		if picks[i] == NoEntity {
			continue
		}
		old := p.Targets
		p.Targets = []EntityID{picks[i]}
		if !containsEntity(old, picks[i]) && !containsEntity(changed, picks[i]) {
			changed = append(changed, picks[i])
		}
	}
	return changed, nil
}
