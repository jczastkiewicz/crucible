package engine

//enginelint:allow id card game player ability condition control amount parts event zone effecthelpers triggerhooks

import (
	"fmt"
	"slices"
)

// proliferateEffect is CountersProliferateEffect.java: Amount$ times
// (default 1), the activator chooses any number of players and battlefield
// permanents that have a counter (ChooseEntitiesForEffect), and each chosen
// one gets one more of every kind of counter it already has (CR 701.34a).
// Players come first, then permanents, as Java's own list.addAll order.
//
// Event$ Proliferate is asked once, before the first pass, with the pass
// count as Num (ReplaceProliferate; Tekuthal's "proliferate twice instead"),
// through playerAmountReplaced. Each counter goes through the AddCounter
// replacements and the CounterAdded triggers (GameEntityCounterTable.
// replaceCounterEffect: Hardened Scales, Winding Constrictor), one kind on one
// entity at a time, as putCounterEffect places them. Mode$ Proliferate fires
// after each of the passes.
type proliferateEffect struct{}

func (proliferateEffect) Resolve(g *Game, a *Ability, controller PlayerController) error {
	if err := rejectUnreadCondition(a, a.Params.Name); err != nil {
		return err
	}
	source := g.Card(a.Source)
	if !subAbilityConditionMet(g, source, a.Amounts, a.Params) {
		return nil
	}
	num := 1
	if raw, ok := a.Params.Param("Amount"); ok {
		n, ok := resolveNamedAmount(g, a.Amounts, source, raw)
		if !ok {
			return fmt.Errorf("engine: Proliferate: Amount$ %q not resolvable yet", raw)
		}
		num = n
	}
	if num > 0 {
		// CountersProliferateEffect.java:40-51; ReplaceProliferate.canReplace
		// refuses a count of 0.
		n, replaced := g.playerAmountReplaced(controller, "Proliferate", "Num", a.Controller, num)
		if replaced {
			return nil
		}
		num = n
	}
	for i := 0; i < num; i++ {
		var options []EntityID
		for _, pid := range g.Players() {
			if len(g.Player(pid).Counters.Kinds()) > 0 {
				options = append(options, PlayerEntity(pid))
			}
		}
		for _, pid := range g.Players() {
			for _, cid := range g.Zone(Battlefield, pid).Cards() {
				if len(g.Card(cid).Counters.Kinds()) > 0 {
					options = append(options, CardEntity(cid))
				}
			}
		}
		chosen := controller.ChooseEntitiesForEffect(g, a.Controller, a.Source, options, 0, len(options))
		if err := checkChoice(chosen, options, 0, len(options)); err != nil {
			return fmt.Errorf("engine: Proliferate: %w", err)
		}
		for _, e := range chosen {
			if pid, ok := e.AsPlayer(); ok {
				for _, kind := range slices.Clone(g.Player(pid).Counters.Kinds()) {
					g.putPlayerCounters(controller, a.Controller, a.Source, pid, kind, 1)
				}
				continue
			}
			cid, _ := e.AsCard()
			for _, kind := range slices.Clone(g.Card(cid).Counters.Kinds()) {
				if n := g.countersReplaced(controller, a.Controller, e, kind, 1); n > 0 {
					g.addCardCounters(controller, a.Controller, a.Source, cid, kind, n)
				}
			}
		}
		g.checkProliferateTriggers(controller, a.Controller)
	}
	return nil
}
