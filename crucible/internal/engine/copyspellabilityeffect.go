package engine

//enginelint:allow ability action amount card castspell condition control defined effecthelpers game id parts player stack targeting trigger triggerpack zone copypermanenteffect

import (
	"fmt"
	"strings"
)

// copySpellAbilityEffect is CopySpellAbilityEffect.java (CR 707.10): for
// each Controller$ player (default You), Optional$ lets them decline each
// named spell, then Amount$ (default 1) copies of it go on the stack under
// that player, above whatever is resolving, so they resolve first. A copy
// is not cast (no SpellCast, no SpellsCastThisTurn, no cast triggers), pays
// no cost, and keeps the original's modes and targets unless
// MayChooseTarget$ lets the copier choose new ones (CR 707.10c). Each copy
// is a new stack object with its own StackItemID, on a new card
// (copySpellHost, Java's COPIED_SPELL) that ceases to exist when it leaves
// the stack (ceaseCopiedSpell, game.go) or becomes a token when it resolves
// as a permanent (copyBecomesToken, castspell.go).
//
// The spells are Defined$ TriggeredSpellAbility -- the stack item a Mode$
// SpellCast trigger recorded (triggeredObjects.spellAbility; 164 of 255
// lines) -- or the ability's own targets (ValidTgts$, spells on the stack,
// targetChoiceFor) and Defined$ Targeted. RememberNewCard$ remembers each
// copy's card on the host; IgnoreFreeze$ and Secondary$ change nothing
// (this port never freezes the stack; Secondary$ is a description flag).
//
// Ported from forge-game/src/main/java/forge/game/ability/effects/CopySpellAbilityEffect.java's
// resolve (CopySpellAbilityEffect.java:65-210) and CardFactory.java's
// copySpellHost/copySpellAbilityAndPossiblyHost (CardFactory.java:80-167).
type copySpellAbilityEffect struct{}

// copySpellUnresolvedParams are CopySpellAbilityEffect params this port does
// not resolve, each rejected before anything is copied (PORT-8, GO-7):
// CopyForEachCanTarget$/ChooseOnlyOne$ (CR 707.10d, one copy per other
// legal target), DefinedTarget$ (CR 707.10e), SingleChoice$, Epic$,
// UseOriginalHost$,
// and Condition$/ConditionDefined$, which subAbilityConditionMet reads as
// never met, silently. The copy's Layer 1 changes (SetColor$, SetPower$,
// SetToughness$, AddTypes$) go through copyExceptDef (CardFactory.
// getCloneStates, :480-612). RememberCopies$ writes SpellAbility objects
// (CopySpellAbilityEffect.java:208-210) that only a spell reader takes
// (ChangeTargets Defined$ Remembered, AbilityUtils.java:1230-1238): it is
// accepted and does nothing here, since Memory holds no SpellAbility.
var copySpellUnresolvedParams = [...]string{
	"CopyForEachCanTarget", "ChooseOnlyOne", "DefinedTarget", "SingleChoice",
	"Epic", "UseOriginalHost"}

func (copySpellAbilityEffect) Resolve(g *Game, a *Ability, controller PlayerController) error {
	if err := rejectUnreadCondition(a, a.Params.Name); err != nil {
		return err
	}
	if err := rejectParams(a, "CopySpellAbility", copySpellUnresolvedParams[:]...); err != nil {
		return err
	}
	source := g.Card(a.Source)
	if !subAbilityConditionMet(g, source, a.Amounts, a.Params) {
		return nil
	}
	amount, err := optionalAmount(g, a, "CopySpellAbility", "Amount", 1)
	if err != nil {
		return err
	}
	spells, err := copySpellTargets(g, a)
	if err != nil {
		return err
	}
	if len(spells) == 0 || amount == 0 {
		return nil
	}
	if battlefieldReplacementEvent(g, "CopySpell") || battlefieldStaticMode(g, "CantBeCopied") {
		return fmt.Errorf("engine: CopySpellAbility: CopySpell replacements or CantBeCopied statics not resolvable yet")
	}
	if g.copyTriggerWatching() {
		return fmt.Errorf("engine: CopySpellAbility: Mode$ SpellCopy/SpellCastOrCopy/SpellAbilityCopy triggers not resolvable yet")
	}
	copierSpec, ok := a.Params.Param("Controller")
	if !ok {
		copierSpec = "You"
	}
	copiers, err := definedPlayers(g, a.Controller, a.Source, copierSpec, a.refs())
	if err != nil {
		return fmt.Errorf("engine: CopySpellAbility: Controller$: %w", err)
	}
	optional := hasParam(a, "Optional")
	mayChoose := hasParam(a, "MayChooseTarget")
	// Every copy goes on the stack before any BecomesTarget trigger a copy's
	// targets raise: Java's MagicStack.add only hands those to the trigger
	// handler, whose abilities reach the stack when a player next receives
	// priority (addSimultaneousStackEntry), so under Amount$ 2 both copies
	// sit below every such trigger.
	var targeted []copiedTargets
	for _, copier := range copiers {
		for _, orig := range spells {
			if optional && !controller.ConfirmEffect(g, copier, a.Source) {
				continue
			}
			if !orig.spell && hasParam(a, "RememberNewCard") {
				return fmt.Errorf("engine: CopySpellAbility: RememberNewCard$ over a copied ability not resolvable yet")
			}
			for i := 0; i < amount; i++ {
				host, tg, err := g.copySpell(controller, a, orig, copier, mayChoose)
				if err == nil && host != NoCard && hasParam(a, "NonLegendary") {
					// CardFactory.java:580, read from the CopySpellAbility, only
					// for a spell copy: the copy's host loses Legendary.
					g.Card(host).copyNonLegendary = true
				}
				if err != nil {
					return err
				}
				if raw, ok := a.Params.Param("SetLoyalty"); ok && host != NoCard {
					// CardFactory.java:614-615: a planeswalker copy starts with
					// this loyalty (Casualty X's amount).
					n, ok := resolveNamedAmount(g, a.Amounts, g.Card(a.Source), raw)
					if !ok {
						return fmt.Errorf("engine: CopySpellAbility: SetLoyalty$ %q is not resolvable", raw)
					}
					g.Card(host).copyLoyalty, g.Card(host).copyHasLoyalty = n, true
				}
				if len(tg.targets) > 0 {
					targeted = append(targeted, tg)
				}
				if hasParam(a, "RememberNewCard") || hasParam(a, "RememberCopies") && host != NoCard {
					// RememberCopies$ remembers the copy's SpellAbility in Java; its
					// card stands for it here (a reader of Remembered spells,
					// ChangeTargets, finds the copy's stack item from the card).
					// Re-read: NewCard (inside copySpell) may have grown the
					// arena and moved g.cards, so source (taken before the
					// loop) can point into a stale backing array whose
					// lazily-allocated Memory.remembered writes are lost
					// (effecteffect.go:143's own convention).
					g.Card(a.Source).Memory.Remember(CardEntity(host))
				}
			}
		}
	}
	for _, tg := range targeted {
		// isSpellSource as castInstantOrSorcery/castAura pass it: true only
		// for an Aura (becomesTargetSourceMatches reads .Aura as always true
		// under a spell source).
		matches := g.checkBecomesTargetTriggers(tg.targets, tg.aura, tg.copier)
		if tg.item != NoStackItem {
			matches = append(matches, g.checkWardTriggersForAbility(tg.targets, tg.item, tg.source, tg.copier)...)
		} else {
			matches = append(matches, g.checkWardTriggers(tg.targets, tg.host, tg.copier)...)
		}
		// The copy's targets are all a crime check reads (isCrime).
		matches = append(matches, g.crimeTriggers(Ability{Controller: tg.copier, Targets: tg.targets})...)
		g.pushTriggeredAbilities(controller, matches)
	}
	return nil
}

// copiedTargets is one pushed copy's targets, held until every copy of the
// resolution is on the stack, for its BecomesTarget check.
type copiedTargets struct {
	targets []EntityID
	aura    bool
	copier  PlayerID
	// host is the copy's card on the stack, what Ward's Counter counters.
	host CardID
	// item and source name a copied activated or triggered ability instead
	// (host is NoCard): Ward counters the ability's stack item.
	item   StackItemID
	source CardID
}

// copySpellTargets is getTargetSpells(sa): the spells a names, as stack
// items by value, in order, CantCopy$ ones dropped (SpellAbility.
// cantBeCopied). Each must still be on the stack.
func copySpellTargets(g *Game, a *Ability) ([]Ability, error) {
	var ids []StackItemID
	if _, ok := a.Params.Param("ValidTgts"); ok {
		// A TargetType$ past the literal Spell (Strionic Resonator's
		// Triggered.YouCtrl) was judged by targeting (stackAbilityCandidates);
		// its targets are spells as cards and abilities as stack items.
		ids = stackItemsOf(g, a.Targets)
	} else {
		defined, _ := a.Params.Param("Defined")
		switch defined {
		case "TriggeredSpellAbility":
			if a.triggered.spellAbility == NoStackItem {
				return nil, fmt.Errorf("engine: CopySpellAbility: Defined$ TriggeredSpellAbility: no triggering spell recorded")
			}
			ids = []StackItemID{a.triggered.spellAbility}
		case "Targeted":
			ids = stackItemsOf(g, a.Targets)
		case "Parent":
			// AbilityUtils.java:1232-1233: the stack root the chain resolves
			// under. It has left the stack as it resolves, so the copy is made
			// from the by-value snapshot resolveTop took (Java holds the
			// spell object).
			if a.rootAbility == nil {
				return nil, fmt.Errorf("engine: CopySpellAbility: Defined$ Parent: no root ability recorded")
			}
			root := *a.rootAbility
			if !root.spell {
				// copySpell builds a spell copy; an activated or triggered
				// root keeps its host (CR 707.10), not built.
				return nil, fmt.Errorf("engine: CopySpellAbility: Defined$ Parent under an ability that is not a spell not resolvable yet")
			}
			if root.Params != nil {
				if _, ok := root.Params.Param("CantCopy"); ok {
					return nil, nil
				}
			}
			return []Ability{root}, nil
		default:
			// Remembered, Imprinted, ValidStack and TriggeredSourceSA are
			// getDefinedSpellAbilities shapes of their own.
			return nil, fmt.Errorf("engine: CopySpellAbility: Defined$ %q not resolvable yet", defined)
		}
	}
	var out []Ability
	for _, id := range ids {
		item, ok := g.stackItem(id)
		if !ok {
			// Java copies from the spell object it still holds (last-known
			// information); this port keeps no stack item past its leaving.
			return nil, fmt.Errorf("engine: CopySpellAbility: copying a spell no longer on the stack not resolvable yet")
		}
		if item.Params != nil {
			if _, ok := item.Params.Param("CantCopy"); ok {
				continue
			}
		}
		out = append(out, *item)
	}
	return out, nil
}

// stackItemsOf is the stack items targets name, in target order: a spell as
// its card, an activated or triggered ability as its stack item (ADR-0041).
// A target that has left the stack is skipped: it has resolved, and there is
// nothing to copy.
func stackItemsOf(g *Game, targets []EntityID) []StackItemID {
	var ids []StackItemID
	for _, t := range targets {
		if id, ok := t.AsStackItem(); ok {
			if _, on := g.stackItem(id); on {
				ids = append(ids, id)
			}
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
	return ids
}

// copyTriggerWatching reports whether any trait host has a trigger copying
// a spell would have to fire (MagicStack.add's TriggerType.SpellCopy,
// SpellAbilityCopy and SpellCastOrCopy runs, MagicStack.java:442-450) --
// none of the three modes is built.
func (g *Game) copyTriggerWatching() bool {
	for _, pid := range g.Players() {
		for _, id := range g.traitHosts(pid) {
			h := g.Card(id)
			if h.Def == nil {
				continue
			}
			for face := range h.triggerFaces {
				for _, t := range face.Triggers {
					switch strings.ToLower(t.Name) {
					case "spellcopy", "spellcastorcopy", "spellabilitycopy":
						return true
					}
				}
			}
		}
	}
	return false
}

// copySpell is CardFactory.copySpellAbilityAndPossiblyHost plus
// orderAndPlaySimultaneousSa's stack add, for one copy of orig under
// copier: a new card from the original's definition, owned by copier,
// straight into the stack zone and marked IsCopiedSpell (copySpellHost,
// CardFactory.java:80-118); the original's API, params, amounts, Charm modes
// and targets carried over (CR 707.10); no cost; then MayChooseTarget$'s new
// targets (CR 707.10c) and the push, which hands the copy its own
// StackItemID. Nothing cast-shaped runs (CR 707.10: not cast); the copy's
// targets come back for the caller's BecomesTarget check, which
// MagicStack.add raises for every added item.
//
// The original's card never carries a Layer 1 copy effect here: copy
// effects end when a permanent leaves the battlefield (endCopiesOnLeave), so
// its printed Def is its copiable values.
func (g *Game) copySpell(controller PlayerController, a *Ability, orig Ability, copier PlayerID, mayChoose bool) (CardID, copiedTargets, error) {
	if !orig.spell {
		return g.copyStackAbility(controller, orig, copier, mayChoose)
	}
	source := g.Card(orig.Source)
	if source.roomDef != nil {
		// A Room spell's own Def, while on the stack, is doorView's single-half
		// projection (room.go) -- not the split card CardEntity's own
		// isRoomDef/roomDef machinery expects. Copying it with NewCard(Def, ...)
		// as-is would build a permanent copy of one bare half that can never be
		// a Room again (enterRoom sees isRoomDef(Def) false), the identical
		// "one-way loss of Room-ness" Clone and Discover are already rejected
		// for (room.go) -- rejected here too rather than
		// silently building a corrupted copy (GO-7/PORT-8).
		return NoCard, copiedTargets{}, fmt.Errorf("engine: CopySpellAbility: copying a Room spell not resolvable yet")
	}
	def, err := copyExceptDef(g, a, source.Def)
	if err != nil {
		return NoCard, copiedTargets{}, fmt.Errorf("engine: CopySpellAbility: %w", err)
	}
	host := g.NewCard(def, copier, Stack)
	g.Card(host).IsCopiedSpell = true
	cp := orig
	cp.ID = NoStackItem
	cp.Source, cp.Controller = host, copier
	cp.spell, cp.Optional = true, false
	cp.hostTransforms, cp.hasHostTransforms = 0, false
	cp.damageMap, cp.replacing = nil, nil
	cp.Targets = append([]EntityID(nil), orig.Targets...)
	// The copy's SubAbility$ links keep the original's targets too (Java
	// copies the whole chain, TargetChoices included), on arrays of their own
	// so stamping or retargeting the copy never writes the original's.
	cp.chainTargets = clonedChainTargets(orig.chainTargets)
	if orig.Modes != nil {
		cp.Modes = make([]Ability, len(orig.Modes))
		for i, m := range orig.Modes {
			m.Source, m.Controller = host, copier
			m.Targets = append([]EntityID(nil), m.Targets...)
			m.chainTargets = clonedChainTargets(m.chainTargets)
			cp.Modes[i] = m
		}
	}
	if mayChoose {
		// setupNewTargets walks the whole chain (SpellAbility.java:2120-2139):
		// the head, its links, then each mode (chained as sub-abilities by
		// CharmEffect.chainAbilities) and that mode's links.
		if err := g.chooseCopyChainTargets(controller, &cp); err != nil {
			return NoCard, copiedTargets{}, err
		}
		for i := range cp.Modes {
			if err := g.chooseCopyChainTargets(controller, &cp.Modes[i]); err != nil {
				return NoCard, copiedTargets{}, err
			}
		}
	}
	g.PushAbility(cp)
	targets := allTargetsOf(cp)
	if cp.Target != NoCard {
		targets = append(targets, CardEntity(cp.Target))
	}
	return host, copiedTargets{targets: targets, aura: cp.API == APIAttach, copier: copier, host: host}, nil
}

// copyStackAbility is copySpell for an activated or triggered ability
// (CopySpellAbilityEffect.java:150-200, CR 707.10): the copy keeps the
// original's host, a card that is not new, and its parameters, targets,
// modes and X; only its controller is the copier. No cost, not activated or
// triggered again, so neither an activation nor a trigger event is raised.
func (g *Game) copyStackAbility(controller PlayerController, orig Ability, copier PlayerID, mayChoose bool) (CardID, copiedTargets, error) {
	cp := orig
	cp.ID = NoStackItem
	cp.Controller = copier
	cp.Optional = false
	cp.damageMap, cp.replacing = nil, nil
	cp.Targets = append([]EntityID(nil), orig.Targets...)
	cp.chainTargets = clonedChainTargets(orig.chainTargets)
	if orig.Modes != nil {
		cp.Modes = make([]Ability, len(orig.Modes))
		for i, m := range orig.Modes {
			m.Controller = copier
			m.Targets = append([]EntityID(nil), m.Targets...)
			m.chainTargets = clonedChainTargets(m.chainTargets)
			cp.Modes[i] = m
		}
	}
	if mayChoose {
		if err := g.chooseCopyChainTargets(controller, &cp); err != nil {
			return NoCard, copiedTargets{}, err
		}
		for i := range cp.Modes {
			if err := g.chooseCopyChainTargets(controller, &cp.Modes[i]); err != nil {
				return NoCard, copiedTargets{}, err
			}
		}
	}
	g.PushAbility(cp)
	targets := allTargetsOf(cp)
	return NoCard, copiedTargets{targets: targets, copier: copier, item: g.nextStackItemID, source: cp.Source}, nil
}

// chooseCopyChainTargets is chooseCopyTargets for m and then each of its
// SubAbility$ links that chose targets, in chain order. A link is asked about
// as a part of its own: its ValidTgts$, evaluated for the copier.
func (g *Game) chooseCopyChainTargets(controller PlayerController, m *Ability) error {
	if err := g.chooseCopyTargets(controller, m); err != nil {
		return err
	}
	for i := range m.chainTargets {
		ct := &m.chainTargets[i]
		link := Ability{Source: m.Source, Controller: m.Controller, Params: ct.Params, Targets: ct.Targets,
			Amounts: m.Amounts, TriggerRemembered: m.TriggerRemembered, triggered: m.triggered, charmRoot: m.charmRoot,
			choosing: m.choosing, hasChoosing: m.hasChoosing}
		if api, ok := APIByName(ct.Params.Name); ok {
			link.API = api
		}
		if err := g.chooseCopyTargets(controller, &link); err != nil {
			return err
		}
		ct.Targets = link.Targets
	}
	return nil
}

// chooseCopyTargets is SpellAbility.setupNewTargets for one targeting part
// of a copy (SpellAbility.java:2133-2152, CR 707.10c): the copier may keep
// its targets or choose new ones, from the same scan and bounds casting
// chose from (targetChoiceFor), evaluated for the copier. Two existing
// decisions, no new one: ConfirmEffect ("choose new targets?"), then
// ChooseTargets on a yes. An Aura copy's own target is the same choice over
// its enchant targets (ChooseEnchantTarget). A part with no targets, or no
// legal candidate now, keeps what it has.
func (g *Game) chooseCopyTargets(controller PlayerController, m *Ability) error {
	if m.API == APIAttach && m.Target != NoCard {
		spec, ok := enchantSpec(g.Card(m.Source))
		if !ok {
			return nil
		}
		eligible := g.enchantTargets(spec, m.Controller, m.Source, false)
		if len(eligible) == 0 || !controller.ConfirmEffect(g, m.Controller, m.Source) {
			return nil
		}
		chosen := controller.ChooseEnchantTarget(g, m.Controller, m.Source, eligible)
		if err := checkChoice([]CardID{chosen}, eligible, 1, 1); err != nil {
			return fmt.Errorf("engine: CopySpellAbility: new Aura target: %w", err)
		}
		m.Target = chosen
		return nil
	}
	if len(m.Targets) == 0 || m.Params == nil {
		return nil
	}
	choice, named, ok := g.targetChoiceFor(m)
	if choice.err != nil {
		return choice.err
	}
	if !named || !ok || len(choice.candidates) == 0 || !controller.ConfirmEffect(g, m.Controller, m.Source) {
		// Kept targets keep their division too (copySpell copies it).
		return nil
	}
	if m.isDivided() {
		// New targets for a divided part keep the original's division values,
		// reassigned to them (PlayerControllerHuman.java:2549, chooseTargets
		// with divisionValues): a decision this port does not ask.
		return fmt.Errorf("engine: CopySpellAbility: new targets for a DividedAsYouChoose$ part not resolvable yet")
	}
	decider, err := g.targetingPlayerOf(m)
	if err != nil {
		return err
	}
	chosen := controller.ChooseTargets(g, decider, choice.candidates, choice.min, choice.max)
	if err := checkChoice(chosen, choice.candidates, choice.min, choice.max); err != nil {
		return fmt.Errorf("engine: CopySpellAbility: new targets: %w", err)
	}
	m.Targets = chosen
	return nil
}
