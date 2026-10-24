package engine

//enginelint:allow game ability control effecthelpers card condition parts zone id destroyeffect

import (
	"fmt"
	"strings"
)

// counterEffect is CounterEffect.java: each targeted spell (a card on the
// stack) is countered -- unless it can't be -- and removed from the stack, its
// card going to Destination$: Graveyard (the default), Exile, Hand,
// TopOfLibrary or BottomOfLibrary. Each targeted activated or triggered
// ability (StackItemEntity, ADR-0041) is removed from the stack and nothing
// moves (CounterEffect.java:270-272); DestroyPermanent$ then destroys its host
// (:105-107). RememberCountered$ and RememberForCounter$ remember the card --
// an ability's host -- on the host of this ability (the latter even if it
// could not be countered). RememberCounteredCMC$ remembers the spell's mana
// value as a number, before the counterable check. Optional$ lets the activator stop first. A Layer$
// CantHappen Counter replacement (counterCantHappen, replacementevents.go)
// stops the counter, and a ReplaceWith$ one replaces a spell's (counterReplaced).
// TargetValidTargeting$ is a targeting restriction (canTargetStackItem), read
// as targets are chosen and again at resolution. Defined$ TriggeredSpellAbility
// and Targeted name the spells, ValidStack<spec> every stack item matching it
// (counterDefinedTargets); any other Defined$,
// any other Counter replacement and a CantBeCountered static are not resolved.
type counterEffect struct{}

func (counterEffect) Resolve(g *Game, a *Ability, controller PlayerController) error {
	if err := rejectUnreadCondition(a, a.Params.Name); err != nil {
		return err
	}
	if err := rejectParams(a, "Counter", "RememberSplicedOntoCounteredSpell", "ConditionWouldDestroy"); err != nil {
		return err
	}
	source := g.Card(a.Source)
	if !subAbilityConditionMet(g, source, a.Amounts, a.Params) {
		return nil
	}
	// A targeted Counter resolves under any TargetType$: targeting offers a
	// spell as its card and an ability as its stack item
	// (stackAbilityCandidates), each countered its own way below. An
	// untargeted one must name Spell: it has no other way to its spell.
	defined, hasDefined := a.Params.Param("Defined")
	if hasDefined && hasParam(a, "ValidTgts") {
		// getSpells(false, "Defined"): a targeting ability ignores Defined$
		// (SpellAbilityEffect.java getSpells), whatever its value.
		hasDefined = false
	}
	if tt, ok := a.Params.Param("TargetType"); !hasDefined && (!ok || tt != "Spell" && !hasParam(a, "ValidTgts")) {
		return fmt.Errorf("engine: Counter: TargetType$ %q not resolvable yet", tt)
	}
	if battlefieldStaticMode(g, "CantBeCountered") {
		return fmt.Errorf("engine: Counter: CantBeCountered statics not resolvable yet")
	}
	dest, ok := a.Params.Param("Destination")
	if !ok {
		dest = "Graveyard"
	}
	if a.wardItem != 0 {
		// Ward on an ability: that stack item, countered as any targeted
		// ability is (counterAbility).
		return counterAbility(g, a, controller, StackItemEntity(a.wardItem))
	}
	targets := a.Targets
	if hasDefined {
		var err error
		if targets, err = counterDefinedTargets(g, a, defined); err != nil {
			return err
		}
	}
	if a.wardCounters != NoEntity {
		// checkWardTriggers' own Defined$ TriggeredSourceSA stand-in
		// (ability.go's wardCounters doc comment, ADR-0028): the spell
		// this Ward-built Counter counters, kept off Targets so it never
		// ran through BecomesTarget or the CR 608.2b fizzle check.
		targets = append(append([]EntityID(nil), targets...), a.wardCounters)
	}
	for _, t := range targets {
		if _, isItem := t.AsStackItem(); isItem {
			if hasParam(a, "Optional") && !controller.ConfirmEffect(g, a.Controller, a.Source) {
				return nil
			}
			if hasParam(a, "RememberCounteredCMC") {
				return fmt.Errorf("engine: Counter: RememberCounteredCMC$ on an ability not resolvable yet")
			}
			if err := counterAbility(g, a, controller, t); err != nil {
				return err
			}
			continue
		}
		id, isCard := t.AsCard()
		if !isCard {
			continue
		}
		if hasParam(a, "Optional") && !controller.ConfirmEffect(g, a.Controller, a.Source) {
			return nil
		}
		c := g.Card(id)
		// Remembered before the counterable check, so even a spell that
		// can't be countered leaves its mana value (CounterEffect.java:
		// 74-79); Count$RememberedNumber reads it back.
		if hasParam(a, "RememberCounteredCMC") {
			source.Memory.RememberNumber(c.CMC())
		}
		if hasParam(a, "RememberForCounter") {
			source.Memory.Remember(t)
		}
		if c.Zone != Stack || c.HasKeyword("This spell can't be countered.") {
			continue
		}
		// CounterEffect.removeFromStack's cantHappenCheck over Event$ Counter.
		cant, err := g.counterCantHappen(id, causeSpell)
		if err != nil {
			return err
		}
		if cant {
			continue
		}
		// Event$ Counter with a ReplaceWith$ ability (Guile): the spell is
		// not countered, the ability acts on it instead.
		if g.counterReplaced(controller, a, id, causeSpell) {
			continue
		}
		// CounterEffect.removeFromStack's game.getStack().remove(si)
		// (CounterEffect.java:258): the spell's own item leaves the stack, and
		// nothing else -- a cast trigger whose source is the spell's card stays
		// and still resolves (MagicStack.remove(Card), MagicStack.java:799-805,
		// keeps abilities the same way).
		g.removeSpellFromStack(id)
		spellDest, err := counterDestination(g, a, controller, dest)
		if err != nil {
			return err
		}
		switch spellDest {
		case "Graveyard":
			g.moveByEffect(controller, id, Graveyard, 0, NoPlayer, false)
		case "Exile":
			g.moveByEffect(controller, id, Exile, 0, NoPlayer, false)
		case "Hand":
			g.moveByEffect(controller, id, Hand, 0, NoPlayer, false)
		case "TopOfLibrary":
			g.moveByEffect(controller, id, Library, 0, NoPlayer, false)
		case "BottomOfLibrary":
			g.moveByEffect(controller, id, Library, -1, NoPlayer, false)
		default:
			return fmt.Errorf("engine: Counter: Destination$ %q not resolvable", spellDest)
		}
		if hasParam(a, "RememberCountered") || hasParam(a, "RememberCounteredSA") {
			// RememberCounteredSA$ remembers the countered SpellAbility
			// (CounterEffect.java:112); a stack object has one entity here.
			source.Memory.Remember(t)
		}
	}
	return nil
}

// counterAbility is CounterEffect.resolve's loop body (CounterEffect.java:
// 72-114) for t, an activated or triggered ability on the stack: RememberForCounter$
// remembers its host card whether or not it is still there; an ability that
// already left the stack is skipped (getInstanceMatchingSpellAbilityID's null,
// :87-90); otherwise a Counter replacement that stops it (counterCantHappen
// with its host as Affected, :252-257) leaves it, and it is removed from the
// stack by its own stack item alone -- a cast trigger whose source is a spell's
// card stays with the spell -- moving no card (:270-272). DestroyPermanent$
// then destroys its host (:105-107) and RememberCountered$ remembers the host
// (:109-110). An ability is always counterable: isCounterableBy is asked of
// a spell only (:83).
func counterAbility(g *Game, a *Ability, controller PlayerController, t EntityID) error {
	ref, known := g.stackTargetOf(t, a.stackTargets)
	if hasParam(a, "RememberForCounter") {
		if !known {
			return fmt.Errorf("engine: Counter: RememberForCounter$ for an ability no longer on the stack not resolvable yet")
		}
		g.Card(a.Source).Memory.Remember(CardEntity(ref.host))
	}
	id, _ := t.AsStackItem()
	item, onStack := g.stackItem(id)
	if !onStack {
		return nil
	}
	host, kind := item.Source, item.causeKind()
	cant, err := g.counterCantHappen(host, kind)
	if err != nil {
		return err
	}
	if cant || g.counterReplaced(controller, a, host, kind) {
		return nil
	}
	g.removeStackItem(id)
	if _, err := counterDestination(g, a, controller, ""); err != nil {
		return err
	}
	if hasParam(a, "RememberCounteredSA") {
		g.Card(a.Source).Memory.Remember(t)
	}
	if hasParam(a, "DestroyPermanent") {
		// GameAction.destroy with regeneration allowed: the steps destroyEffect
		// runs per card (destroyeffect.go).
		if c := g.Card(host); c.Zone == Battlefield && canBeDestroyed(c) &&
			!g.destroyInstead(controller, host) && !g.regenerate(controller, host) {
			g.moveToGraveyard(controller, host)
			g.checkDiesTriggers(controller, host)
			g.checkChangesZoneAllTriggers(controller, []CardID{host}, Battlefield, Graveyard)
		}
	}
	if hasParam(a, "RememberCountered") {
		g.Card(a.Source).Memory.Remember(CardEntity(host))
	}
	return nil
}

// removeStackItem takes the item whose ID is id off the stack, and nothing
// else: MagicStack.remove(SpellAbilityStackInstance).
func (g *Game) removeStackItem(id StackItemID) {
	kept := g.stack[:0]
	for _, s := range g.stack {
		if s.ID != id {
			kept = append(kept, s)
		}
	}
	clear(g.stack[len(kept):])
	g.stack = kept
}

// counterDefinedTargets is getTargetSpells(sa) for an untargeted Counter
// (AbilityUtils.getDefinedSpellAbilities, :1234-1266): Defined$
// TriggeredSpellAbility is the stack item the trigger recorded, Targeted the
// parent's targets. An item that left the stack is dropped, as CounterEffect
// skips an instance it cannot find (:87-90). A spell is handed on as its card,
// an activated or triggered ability as its stack item, the two shapes targeting
// itself produces.
func counterDefinedTargets(g *Game, a *Ability, defined string) ([]EntityID, error) {
	switch defined {
	case "Targeted":
		return a.Targets, nil
	case "Remembered":
		// The spells and abilities the host remembers (RepeatEach
		// RepeatSpellAbilities$); one that left the stack is dropped.
		var out []EntityID
		for _, e := range g.Card(a.Source).Memory.Remembered() {
			if sid, ok := e.AsStackItem(); ok {
				if _, onStack := g.stackItem(sid); onStack {
					out = append(out, e)
				}
			} else if cid, ok := e.AsCard(); ok && g.Card(cid).Zone == Stack {
				out = append(out, e)
			}
		}
		return out, nil
	case "TriggeredSpellAbility":
		if a.triggered.spellAbility == NoStackItem {
			return nil, fmt.Errorf("engine: Counter: Defined$ TriggeredSpellAbility: no triggering spell recorded")
		}
		item, ok := g.stackItem(a.triggered.spellAbility)
		if !ok {
			return nil, nil
		}
		if item.spell {
			return []EntityID{CardEntity(item.Source)}, nil
		}
		return []EntityID{StackItemEntity(item.ID)}, nil
	}
	if restriction, ok := strings.CutPrefix(defined, "ValidStack "); ok {
		// getDefinedSpellAbilities :1279-1287: every stack item, in stack
		// order, that isValid for the restriction.
		var out []EntityID
		for i := range g.stack {
			item := &g.stack[i]
			matched, err := g.stackItemMatches(item, restriction, a)
			if err != nil {
				return nil, fmt.Errorf("engine: Counter: Defined$ %q: %w", defined, err)
			}
			if !matched {
				continue
			}
			if item.spell {
				out = append(out, CardEntity(item.Source))
			} else {
				out = append(out, StackItemEntity(item.ID))
			}
		}
		return out, nil
	}
	return nil, fmt.Errorf("engine: Counter: Defined$ %q not resolvable yet", defined)
}

// counterDestination is where a countered spell goes: Destination$ (default
// Graveyard), or DestinationChoice$'s comma list when given, the activator
// choosing (chooseSomeType, CounterEffect.java:264-268). Java asks it for an
// ability too, before it finds there is no card to move (:270): so does this,
// with dest "" and an answer nothing uses (PORT-7).
func counterDestination(g *Game, a *Ability, controller PlayerController, dest string) (string, error) {
	raw, ok := a.Params.Param("DestinationChoice")
	if !ok {
		return dest, nil
	}
	options := strings.Split(raw, ",")
	i := controller.ChooseOption(g, a.Controller, a.Source, options)
	if i < 0 || i >= len(options) {
		return "", fmt.Errorf("engine: Counter: DestinationChoice$ choice %d out of range", i)
	}
	return strings.TrimSpace(options[i]), nil
}
