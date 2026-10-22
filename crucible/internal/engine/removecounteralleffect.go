// RemoveCounterAll: CR 122.1, removeCounterEffect's own battlefield-sweep
// sibling and PutCounterAll's own mirror image -- 19 real (AB|DB)$
// RemoveCounterAll lines, every one naming both ValidCards$ and
// CounterType$; CounterNum$ is a resolvable amount (6), absent (nothing is
// removed: no default), or AllCounters$ (12) overrides it entirely with the
// target's own current count of that one kind, computed per target the
// identical way removeCounterEffect's own CounterNum$ All does -- a
// distinct literal param here rather than a CounterNum$ value,
// CountersRemoveAllEffect.java's own `sa.hasParam("AllCounters")` check. ValidZone$
// picks the zone swept (Battlefield by default) and AllCounterTypes$ empties
// every kind off each card.
//
// Ported from
// forge-game/src/main/java/forge/game/ability/effects/CountersRemoveAllEffect.java's
// resolve. Reuses removeCounters (removecountereffect.go) outright for the
// per-card body -- the first M6 pack whose own "All" sibling shares its
// per-entity write path with its own singular effect's file directly,
// rather than each effect owning a private copy.

package engine

//enginelint:allow id card game ability condition control zone valid amount removecountereffect parts

import (
	"fmt"
	"strings"

	"github.com/jczastkiewicz/crucible/internal/valid"
)

// removeCounterAllEffect resolves Mode$/DB$/AB$ RemoveCounterAll.
// ConditionPresent$/ConditionCompare$/ConditionCheckSVar$/
// ConditionSVarCompare$ are resolved through subAbilityConditionMet
// (condition.go), the identical way every other M6 effect's own does.
type removeCounterAllEffect struct{}

func (removeCounterAllEffect) Resolve(g *Game, a *Ability, _ PlayerController) error {
	if err := rejectUnreadCondition(a, a.Params.Name); err != nil {
		return err
	}
	source := g.Card(a.Source)
	if !subAbilityConditionMet(g, source, a.Amounts, a.Params) {
		return nil
	}

	validCards, ok := a.Params.Param("ValidCards")
	if !ok {
		return fmt.Errorf("engine: RemoveCounterAll: ValidCards$ missing")
	}
	spec := valid.Parse(validCards)

	zone := Battlefield
	if raw, ok := a.Params.Param("ValidZone"); ok {
		if zone, ok = ZoneByName(raw); !ok {
			return fmt.Errorf("engine: RemoveCounterAll: ValidZone$ %q is not a zone", raw)
		}
	}

	// AllCounterTypes$ (CountersRemoveAllEffect.java:63-69) empties every
	// kind off each card and reads nothing else.
	_, allKinds := a.Params.Param("AllCounterTypes")
	var counterType CounterType
	var allAmount bool
	var amount int
	if !allKinds {
		counterTypeRaw, ok := a.Params.Param("CounterType")
		if !ok {
			return fmt.Errorf("engine: RemoveCounterAll: CounterType$ missing")
		}
		if strings.Contains(counterTypeRaw, ",") || strings.EqualFold(counterTypeRaw, "Any") || strings.EqualFold(counterTypeRaw, "All") {
			return fmt.Errorf("engine: RemoveCounterAll: CounterType$ %q not resolvable yet", counterTypeRaw)
		}
		counterType = CounterType(strings.ToUpper(counterTypeRaw))

		_, allAmount = a.Params.Param("AllCounters")
		if !allAmount {
			// CountersRemoveAllEffect.java:48 reads CounterNum$ with no
			// default, so a line naming neither it nor AllCounters$ removes
			// nothing (corrosion.txt, a script defect).
			if counterNumParam, ok := a.Params.Param("CounterNum"); ok {
				amount, ok = resolveNamedAmount(g, a.Amounts, source, counterNumParam)
				if !ok {
					return fmt.Errorf("engine: RemoveCounterAll: CounterNum$ %q is not resolvable", counterNumParam)
				}
			}
		}
	}

	for _, pid := range g.Players() {
		for _, cid := range g.Zone(zone, pid).Cards() {
			c := g.Card(cid)
			if !Matches(g, c, spec, a.Controller, a.Source) {
				continue
			}
			removeCounters(g, a.Source, CardEntity(cid), &c.Counters, counterType, allKinds, allAmount, amount)
		}
	}
	return nil
}
