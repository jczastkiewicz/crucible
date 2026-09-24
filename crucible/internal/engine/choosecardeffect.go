package engine

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/jczastkiewicz/crucible/internal/valid"
)

// chooseCardUnresolvedParams are ChooseCardEffect.java's params this port
// cannot honour yet: every alternative selection mode (AtRandom$ needs
// Aggregates.random's exact RNG draw order, ChooseEach$/EachBasicType$/
// WithTotalPower$/WithDifferentPowers$/EachDifferentPower$/ControlAndNot$/
// QuasiLibrarySearch$ are separate loops), the non-default choice pools
// (AllCards$, IncludeSpellsOnStack$, TargetControls$), ChosenMap$'s
// per-player map and Secretly$'s hidden reveal. 399 real (AB|DB)$ lines;
// the default chooseCardsForEffect shape covers most of them.
var chooseCardUnresolvedParams = [...]string{
	"AtRandom", "ChooseEach", "EachBasicType", "WithTotalPower",
	"WithDifferentPowers", "EachDifferentPower", "ControlAndNot",
	"QuasiLibrarySearch", "AllCards", "IncludeSpellsOnStack", "TargetControls",
	"ChosenMap", "Secretly", "ChoiceTitleAppend", "StartingWith", "Optional",
	"UnlessResolveSubs", "LockInText", "OrString",
	"Condition", "ConditionDefined", "SorcerySpeed", "PlayerTurn", "ModeCost",
	"ConditionManaNotSpent", "ConditionGameTypes",
}

// chooseCardEffect is ChooseCardEffect.java's default shape: each chooser
// (Defined$/ValidTgts$, default You) picks between MinAmount$ and Amount$
// cards (both default 1) from the ChoiceZone$ cards (default Battlefield)
// matching Choices$, or from DefinedCards$ outright. The union becomes the
// host's chosen cards (Memory.Choose, read back by Defined$ ChosenCard and
// the ChosenCard valid property), then RememberChosen$/ImprintChosen$/
// ForgetChosen$/ForgetOtherRemembered$ update Memory the same way Java's own
// trailing block does. Without Mandatory$ the choice may be empty, Java's
// own isOptional = !Mandatory.
type chooseCardEffect struct{}

func (chooseCardEffect) Resolve(g *Game, a *Ability, controller PlayerController) error {
	for _, key := range chooseCardUnresolvedParams {
		if _, ok := a.Params.Param(key); ok {
			return fmt.Errorf("engine: ChooseCard: %s$ not resolvable yet", key)
		}
	}
	source := g.Card(a.Source)
	if !subAbilityConditionMet(g, source, a.Amounts, a.Params) {
		return nil
	}
	choosers, err := targetedOrDefinedPlayers(g, a.Controller, a.Source, a.Params, a.Targets)
	if err != nil {
		return fmt.Errorf("engine: ChooseCard: %w", err)
	}

	choices, err := chooseCardPool(g, a, source)
	if err != nil {
		return err
	}

	amountValue, ok := a.Params.Param("Amount")
	if !ok {
		amountValue = "1"
	}
	maxAmount, ok := resolveNamedAmount(g, a.Amounts, source, amountValue)
	if !ok {
		return fmt.Errorf("engine: ChooseCard: Amount$ %q not resolvable yet", amountValue)
	}
	minAmount := maxAmount
	if raw, ok := a.Params.Param("MinAmount"); ok {
		n, err := strconv.Atoi(raw)
		if err != nil {
			return fmt.Errorf("engine: ChooseCard: MinAmount$ %q is not an integer", raw)
		}
		minAmount = n
	}
	if maxAmount <= 0 {
		return nil
	}
	if _, mandatory := a.Params.Param("Mandatory"); !mandatory {
		minAmount = 0
	}

	var all []CardID
	for _, pid := range choosers {
		pChoices := choices
		if by, ok := a.Params.Param("ControlledByPlayer"); ok {
			if by != "Chooser" {
				return fmt.Errorf("engine: ChooseCard: ControlledByPlayer$ %q not resolvable yet", by)
			}
			pChoices = nil
			for _, cid := range choices {
				if g.Card(cid).Controller() == pid {
					pChoices = append(pChoices, cid)
				}
			}
		}
		lo, hi := minAmount, maxAmount
		if hi > len(pChoices) {
			hi = len(pChoices)
		}
		if lo > hi {
			lo = hi
		}
		chosen := controller.ChooseCardsForEffect(g, pid, a.Source, pChoices, lo, hi)
		if err := checkChoice(chosen, pChoices, lo, hi); err != nil {
			return fmt.Errorf("engine: ChooseCard: %w", err)
		}
		all = append(all, chosen...)
	}

	m := &source.Memory
	m.ClearChosen()
	for _, cid := range all {
		m.Choose(cid)
	}
	if _, ok := a.Params.Param("ForgetOtherRemembered"); ok {
		m.ClearRemembered()
	}
	if _, ok := a.Params.Param("RememberChosen"); ok {
		for _, cid := range all {
			m.Remember(CardEntity(cid))
		}
	}
	if _, ok := a.Params.Param("ForgetChosen"); ok {
		for _, cid := range all {
			m.Forget(CardEntity(cid))
		}
	}
	if _, ok := a.Params.Param("ImprintChosen"); ok {
		for _, cid := range all {
			m.Imprint(cid)
		}
	}
	return nil
}

// chooseCardPool is the candidate list ChooseCardEffect.java builds before
// asking anyone: DefinedCards$ outright when present, otherwise every card
// in the ChoiceZone$ zones (comma-separated, default Battlefield) filtered
// by Choices$, in seat order per zone -- game.getCardsIn's own order.
func chooseCardPool(g *Game, a *Ability, source *Card) ([]CardID, error) {
	if defined, ok := a.Params.Param("DefinedCards"); ok {
		cards, err := definedCards(source, defined, a.Targets)
		if err != nil {
			return nil, fmt.Errorf("engine: ChooseCard: DefinedCards$: %w", err)
		}
		return cards, nil
	}
	zones := []ZoneType{Battlefield}
	if raw, ok := a.Params.Param("ChoiceZone"); ok {
		zones = nil
		for _, name := range strings.Split(raw, ",") {
			z, ok := ZoneByName(strings.TrimSpace(name))
			if !ok {
				return nil, fmt.Errorf("engine: ChooseCard: ChoiceZone$ %q not resolvable yet", name)
			}
			zones = append(zones, z)
		}
	}
	choicesParam, hasChoices := a.Params.Param("Choices")
	var spec valid.Spec
	if hasChoices {
		spec = valid.Parse(choicesParam)
	}
	var pool []CardID
	for _, z := range zones {
		for _, pid := range g.Players() {
			for _, cid := range g.Zone(z, pid).Cards() {
				if !hasChoices || Matches(g, g.Card(cid), spec, a.Controller, a.Source) {
					pool = append(pool, cid)
				}
			}
		}
	}
	return pool, nil
}

// checkChoice validates a controller's answer against the offer: every
// pick drawn from options, none twice, and a count in [min, max]. A bad
// answer is a controller bug a card script can surface, so it is an error,
// not a panic (GO-7).
func checkChoice[T comparable](chosen, options []T, min, max int) error {
	if len(chosen) < min || len(chosen) > max {
		return fmt.Errorf("controller chose %d, want between %d and %d", len(chosen), min, max)
	}
	offered := make(map[T]bool, len(options))
	for _, o := range options {
		offered[o] = true
	}
	for _, c := range chosen {
		if !offered[c] {
			return fmt.Errorf("controller chose %v, which was not offered", c)
		}
		delete(offered, c)
	}
	return nil
}
