package engine

//enginelint:allow game ability control effecthelpers card condition parts defined id event presentcheck

import (
	"fmt"
	"strings"
)

// addOrRemoveCounterEffect is CountersPutOrRemoveEffect.java: for each
// Defined$ or targeted card, CounterNum$ (default 1) counters of one kind
// are put on it or removed from it. The kind is CounterType$, or -- when
// that is absent, or with EachExistingCounter$ for every kind the card has
// -- one the DefinedPlayer$ (the activator by default) picks among the
// kinds on the card. A card with no counters can only receive CounterType$.
// Whether to add or remove is that player's chooseBinary (AddOrRemove):
// nothing in this port stops a card receiving or losing a counter, so the
// question is always asked. Optional$ lets the player skip each card.
type addOrRemoveCounterEffect struct{}

func (addOrRemoveCounterEffect) Resolve(g *Game, a *Ability, controller PlayerController) error {
	if err := rejectUnreadCondition(a, a.Params.Name); err != nil {
		return err
	}
	source := g.Card(a.Source)
	if !subAbilityConditionMet(g, source, a.Amounts, a.Params) {
		return nil
	}
	n, err := optionalAmount(g, a, "AddOrRemoveCounter", "CounterNum", 1)
	if err != nil {
		return err
	}
	if n <= 0 {
		return nil
	}
	var ctype CounterType
	if raw, ok := a.Params.Param("CounterType"); ok {
		ctype = CounterType(strings.ToUpper(raw))
	}
	decider := a.Controller
	if def, ok := a.Params.Param("DefinedPlayer"); ok {
		players, err := definedPlayers(g, a.Controller, a.Source, def, a.refs())
		if err != nil {
			return fmt.Errorf("engine: AddOrRemoveCounter: %w", err)
		}
		if len(players) == 0 {
			return nil
		}
		decider = players[0]
	}
	cards, err := targetedOrDefinedCards(source, a.Params, a.refs())
	if err != nil {
		return fmt.Errorf("engine: AddOrRemoveCounter: %w", err)
	}
	_, each := a.Params.Param("EachExistingCounter")
	_, remember := a.Params.Param("RememberRemovedCards")
	for _, id := range cards {
		c := g.Card(id)
		if hasParam(a, "Optional") && !controller.ConfirmEffect(g, decider, a.Source) {
			continue
		}
		if !c.Counters.Any() {
			// CountersPutOrRemoveEffect.java:93-95 adds with no look at
			// RemoveConditionSVar$ when the card holds no counter, so Shape of
			// the Wiitigo grows a creature that did not attack or block: a
			// Forge defect (forge-java-defects.md), refused by name.
			if hasParam(a, "RemoveConditionSVar") {
				return fmt.Errorf("engine: AddOrRemoveCounter: RemoveConditionSVar$ on a card with no counter, where Java adds one regardless (CountersPutOrRemoveEffect.java:93-95)")
			}
			if !each && ctype != "" {
				c.Counters.Add(ctype, n)
				emitCounterChanged(g, a.Source, CardEntity(id), ctype, n)
			}
			continue
		}
		kinds := []CounterType{ctype}
		if each {
			kinds = append([]CounterType(nil), c.Counters.Kinds()...)
		}
		for _, kind := range kinds {
			if err := addOrRemoveCounter(g, a, controller, decider, id, kind, n, remember); err != nil {
				return err
			}
		}
	}
	return nil
}

// addOrRemoveCounter is the effect's per-kind step: with no kind given the
// decider picks one of the card's (chooseCounterType), then chooses to add
// or remove n of it.
func addOrRemoveCounter(g *Game, a *Ability, controller PlayerController, decider PlayerID, id CardID, kind CounterType, n int, remember bool) error {
	c := g.Card(id)
	if kind == "" {
		kinds := c.Counters.Kinds()
		names := make([]string, len(kinds))
		for i, k := range kinds {
			names[i] = string(k)
		}
		i := 0
		if len(kinds) > 1 {
			i = controller.ChooseOption(g, decider, a.Source, names)
			if i < 0 || i >= len(kinds) {
				return fmt.Errorf("engine: AddOrRemoveCounter: counter type choice %d out of range", i)
			}
		}
		kind = kinds[i]
	}
	add, err := g.addOrRemoveDecision(a, controller, decider)
	if err != nil {
		return err
	}
	if add {
		c.Counters.Add(kind, n)
		emitCounterChanged(g, a.Source, CardEntity(id), kind, n)
		return nil
	}
	before := c.Counters.Count(kind)
	removed := before - c.Counters.Add(kind, -n)
	if removed > 0 {
		emitCounterChanged(g, a.Source, CardEntity(id), kind, -removed)
	}
	if remember {
		g.Card(a.Source).Memory.Remember(CardEntity(id))
	}
	return nil
}

// addOrRemoveDecision is CountersPutOrRemoveEffect.java:116-138: with
// RemoveConditionSVar$ the player is never asked; the counter is removed when
// the inline expression (an "AbilityUtils.calculateAmount" string with a '$',
// not an SVar name) reaches RemoveConditionSVarCompare$ (default GE1) and added
// otherwise. Without it the decider chooses.
func (g *Game) addOrRemoveDecision(a *Ability, controller PlayerController, decider PlayerID) (bool, error) {
	if !hasParam(a, "RemoveConditionSVar") {
		return controller.ChooseBinary(g, decider, a.Source, AddOrRemove), nil
	}
	remove := checkSVarMatches(g, g.Card(a.Source), a.Amounts, a.Params, "RemoveConditionSVar", "RemoveConditionSVarCompare", "")
	return !remove, nil
}
