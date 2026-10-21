// MoveCounter: CR 121.5, "move N counters of a kind from one object to
// another" -- 30 real (AB|DB)$ MoveCounter lines, mostly Source$ Self (15)
// or another definedCards-resolvable value (9, 6 of them a value
// (Creature/Permanent/ParentTarget) this port's own definedCards has no
// case for and so fails loudly rather than guessing). Two shapes resolve:
// one Source$ card, one or more ValidTgts$/Defined$ destinations
// (targetedOrDefinedCards, defined.go, default "Self" the identical way
// Java's own getDefinedCardsOrTargeted(sa) with no override does), and
// ValidSource$, many sources to the first destination. CounterNum$ Any asks
// the activator which sources (ChooseCardsForEffect, ValidSource$ only) and how
// many counters of each kind (ChooseNumber, 0 to what the source holds); the
// ValidDefined$ branch and CounterType$ Any fail loudly.
//
// Ported from
// forge-game/src/main/java/forge/game/ability/effects/CountersMoveEffect.java's
// resolve, trimmed hard: that file's own two-target shape (TargetMin$ 2, the
// FIRST target itself the source rather than Source$ naming it) is not
// ported -- see moveCounterUnresolvedParams below.

package engine

//enginelint:allow id card game ability defined condition control amount parts event phaseseffect effecthelpers

import (
	"fmt"
	"strings"
)

// moveCounterUnresolvedParams names CountersMoveEffect's own params this
// port does not evaluate. Every one fails the whole line loudly (PORT-8/
// GO-7): ValidDefined$ (0, defensive) -- the "one source to many
// destinations" branch, listed even at zero real lines rather than silently
// misreading it as a plain Source$ value; NonZero$ (1) -- CounterNum$ Any's own "at least one"
// floor, moot on its own since CounterNum$ Any already fails below.
var moveCounterUnresolvedParams = [...]string{
	"ValidDefined", "NonZero",
}

// moveCounterEffect resolves Mode$/DB$/AB$ MoveCounter for the one-source,
// one-or-more-destinations shape.
type moveCounterEffect struct{}

func (moveCounterEffect) Resolve(g *Game, a *Ability, controller PlayerController) error {
	for _, key := range moveCounterUnresolvedParams {
		if _, ok := a.Params.Param(key); ok {
			return fmt.Errorf("engine: MoveCounter: %s$ not resolvable yet", key)
		}
	}
	source := g.Card(a.Source)
	if !subAbilityConditionMet(g, source, a.Amounts, a.Params) {
		return nil
	}

	counterTypeRaw, ok := a.Params.Param("CounterType")
	if !ok {
		return fmt.Errorf("engine: MoveCounter: CounterType$ missing")
	}
	var counterType CounterType
	allKinds := false
	switch {
	case strings.EqualFold(counterTypeRaw, "All"):
		allKinds = true
	case strings.Contains(counterTypeRaw, ","), strings.EqualFold(counterTypeRaw, "Any"), strings.EqualFold(counterTypeRaw, "EachNotOn"):
		return fmt.Errorf("engine: MoveCounter: CounterType$ %q not resolvable yet", counterTypeRaw)
	default:
		counterType = CounterType(strings.ToUpper(counterTypeRaw))
	}

	counterNumParam, ok := a.Params.Param("CounterNum")
	if !ok {
		counterNumParam = "1"
	}
	allAmount := strings.EqualFold(counterNumParam, "All")
	anyAmount := strings.EqualFold(counterNumParam, "Any")
	var amount int
	if !allAmount && !anyAmount {
		amount, ok = resolveNamedAmount(g, a.Amounts, source, counterNumParam)
		if !ok {
			return fmt.Errorf("engine: MoveCounter: CounterNum$ %q is not resolvable", counterNumParam)
		}
	}

	destCards, err := targetedOrDefinedCards(source, a.Params, a.refs())
	if err != nil {
		return fmt.Errorf("engine: MoveCounter: %w", err)
	}
	if spec, ok := a.Params.Param("ValidSource"); ok {
		// CountersMoveEffect.java:104-162: every battlefield card matching
		// ValidSource$ gives its counters to the first destination; under
		// CounterNum$ Any the activator first picks which of the sources
		// holding the counter (CountersMoveEffect.java:104-148; the
		// destination stays a candidate, Java drops it only in removeCounter).
		if len(destCards) == 0 {
			return nil
		}
		srcCards, err := phasesValidCards(g, a, spec, false)
		if err != nil {
			return fmt.Errorf("engine: MoveCounter: ValidSource$: %w", err)
		}
		dest := destCards[0]
		if anyAmount {
			var holders []CardID
			for _, src := range srcCards {
				c := g.Card(src)
				if (allKinds && c.Counters.Any()) || (!allKinds && c.Counters.Count(counterType) > 0) {
					holders = append(holders, src)
				}
			}
			if srcCards = holders; len(srcCards) > 0 {
				srcCards = controller.ChooseCardsForEffect(g, a.Controller, a.Source, srcCards, 0, len(srcCards))
				if err := checkChoice(srcCards, holders, 0, len(holders)); err != nil {
					return fmt.Errorf("engine: MoveCounter: sources: %w", err)
				}
			}
		}
		for _, src := range srcCards {
			if src != dest {
				moveCounters(g, controller, a.Controller, a.Source, src, dest, counterType, allKinds, allAmount, anyAmount, amount)
			}
		}
		return nil
	}
	var src CardID
	_, targeting := a.Params.Param("ValidTgts")
	if minTargets, _ := a.Params.Param("TargetMin"); targeting && minTargets == "2" {
		// CountersMoveEffect.java:229-233: with two targets the first is
		// the source and the rest receive.
		if len(destCards) < 2 {
			return nil
		}
		src, destCards = destCards[0], destCards[1:]
	} else {
		sourceParam, ok := a.Params.Param("Source")
		if !ok {
			sourceParam = "Self"
		}
		srcCards, err := definedCards(source, sourceParam, a.refs())
		if err != nil {
			return fmt.Errorf("engine: MoveCounter: Source$: %w", err)
		}
		if len(srcCards) == 0 {
			return nil
		}
		src = srcCards[0]
	}
	if !g.Card(src).Counters.Any() {
		return nil
	}

	for _, dest := range destCards {
		if dest == src {
			continue
		}
		moveCounters(g, controller, a.Controller, a.Source, src, dest, counterType, allKinds, allAmount, anyAmount, amount)
	}
	return nil
}

// moveCounters is CountersMoveEffect.resolve's own per-destination body:
// allKinds moves every kind src currently carries (each capped at its own
// count, CountersRemoveEffect's own identical per-kind independence);
// otherwise only counterType moves. placer, the activator, puts them on dest
// (CountersMoveEffect.java:86, :160).
func moveCounters(g *Game, controller PlayerController, placer PlayerID, source, src, dest CardID, counterType CounterType, allKinds, allAmount, anyAmount bool, amount int) {
	srcCounters := &g.Card(src).Counters
	if allKinds {
		for _, kind := range srcCounters.Kinds() {
			moveOneCounterKind(g, controller, placer, source, src, dest, srcCounters, kind, allAmount, anyAmount, amount)
		}
		return
	}
	moveOneCounterKind(g, controller, placer, source, src, dest, srcCounters, counterType, allAmount, anyAmount, amount)
}

func moveOneCounterKind(g *Game, controller PlayerController, placer PlayerID, source, src, dest CardID, srcCounters *Counters, kind CounterType, allAmount, anyAmount bool, amount int) {
	cmax := srcCounters.Count(kind)
	if cmax <= 0 {
		return
	}
	n := amount
	if anyAmount {
		// removeCounter's chooseNumber(min 0, max cmax), CountersMoveEffect.java:355.
		n = controller.ChooseNumber(g, placer, source, 0, cmax)
	}
	if allAmount || n > cmax {
		n = cmax
	}
	if n <= 0 {
		return
	}
	srcCounters.Add(kind, -n)
	emitCounterChanged(g, source, CardEntity(src), kind, -n)
	g.addCardCounters(controller, placer, source, dest, kind, n)
}
