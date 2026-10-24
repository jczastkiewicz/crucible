package engine

//enginelint:allow id card game ability defined condition control amount parts zone valid effecthelpers scheduledaction repeateacheffect

import (
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/jczastkiewicz/crucible/internal/valid"
)

// chooseCardUnresolvedParams are ChooseCardEffect.java's params this port
// cannot honour yet: the alternative selection loops (WithDifferentPowers$/EachDifferentPower$/
// ControlAndNot$; ChooseEach$, EachBasicType$ and WithTotalPower$ resolve, chooseEachCards,
// chooseWithTotalPower), the non-default choice pools (AllCards$,
// IncludeSpellsOnStack$; TargetControls$ resolves, chooseCardPool) and ChosenMap$'s per-player map.
// 399 real (AB|DB)$ lines; the default chooseCardsForEffect shape covers
// most of them. QuasiLibrarySearch$ resolves (chooseCardQuasiSearch);
// AtRandom$ is Aggregates.random(pChoices, Amount) over the pool in its scan
// order (ChooseCardEffect.java:225-228); Secretly$ only moves Reveal$'s
// display to after every chooser (:275-288), no state.
var chooseCardUnresolvedParams = [...]string{
	"IncludeSpellsOnStack",
	"UnlessResolveSubs", "LockInText",
	"ConditionGameTypes",
}

// chooseCardEffect is ChooseCardEffect.java's default shape: each chooser
// (Defined$/ValidTgts$, default You) picks between MinAmount$ and Amount$
// cards (both default 1) from the ChoiceZone$ cards (default Battlefield)
// matching Choices$, or from DefinedCards$ outright. The union becomes the
// host's chosen cards (Memory.Choose, read back by Defined$ ChosenCard and
// the ChosenCard valid property), then RememberChosen$/ImprintChosen$/
// ForgetChosen$/ForgetOtherRemembered$ update Memory the same way Java's own
// trailing block does. Without Mandatory$ the choice may be empty, Java's
// own isOptional = !Mandatory, which is why Optional$ (read by nothing in
// Java, card-script-defects.md) changes nothing. StartingWith$ orders the
// choosers (getPlayers' trailing sort); ChoiceTitleAppend$ only extends the
// prompt title.
type chooseCardEffect struct{}

func (chooseCardEffect) Resolve(g *Game, a *Ability, controller PlayerController) error {
	if err := rejectUnreadCondition(a, a.Params.Name); err != nil {
		return err
	}
	for _, key := range chooseCardUnresolvedParams {
		if _, ok := a.Params.Param(key); ok {
			return fmt.Errorf("engine: ChooseCard: %s$ not resolvable yet", key)
		}
	}
	source := g.Card(a.Source)
	if !subAbilityConditionMet(g, source, a.Amounts, a.Params) {
		return nil
	}
	choosers, err := targetedOrDefinedPlayers(g, a.Controller, a.Source, a.Params, a.refs())
	if err != nil {
		return fmt.Errorf("engine: ChooseCard: %w", err)
	}

	if start, ok := a.Params.Param("StartingWith"); ok {
		if choosers, err = playersStartingWith(g, a, choosers, start); err != nil {
			return err
		}
	}

	choices, err := chooseCardPool(g, a, source)
	if err != nil {
		return err
	}
	if hasParam(a, "TargetControls") && !hasParam(a, "DefinedCards") {
		// CardLists.filterControlledBy(choices, tgtPlayers), before DefinedCards$
		// replaces the pool (ChooseCardEffect.java:73-78); the players are the
		// choosers.
		kept := choices[:0:0]
		for _, cid := range choices {
			if slices.Contains(choosers, g.Card(cid).Controller()) {
				kept = append(kept, cid)
			}
		}
		choices = kept
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
	rawMin := minAmount
	if _, mandatory := a.Params.Param("Mandatory"); !mandatory {
		minAmount = 0
	}

	quasi, err := chooseCardQuasiSearch(g, a, choosers)
	if err != nil {
		return err
	}

	var all []CardID
	// ChosenMap$ (ChooseCardEffect.java:281-283): each chooser's own picks are
	// stored per chooser as that chooser finishes.
	chosenMap := hasParam(a, "ChosenMap")
	mapPid, mapStart := NoPlayer, -1
	flushMap := func() {
		if chosenMap && mapStart >= 0 {
			source.Memory.SetChosenMap(mapPid, slices.Clone(all[mapStart:]))
		}
	}
	for _, pid := range choosers {
		flushMap()
		mapPid, mapStart = pid, len(all)
		pChoices := choices
		if by, ok := a.Params.Param("ControlledByPlayer"); ok {
			var owners []PlayerID
			switch by {
			case "Chooser":
				owners = []PlayerID{pid}
			case "Left", "Right":
				owners = []PlayerID{g.nextPlayerInDirection(pid, by == "Right")}
			default:
				owners, err = definedPlayers(g, a.Controller, a.Source, by, a.refs())
				if err != nil {
					return fmt.Errorf("engine: ChooseCard: ControlledByPlayer$: %w", err)
				}
			}
			pChoices = nil
			for _, cid := range choices {
				if slices.Contains(owners, g.Card(cid).Controller()) {
					pChoices = append(pChoices, cid)
				}
			}
		}
		if raw, ok := a.Params.Param("WithTotalPower"); ok {
			chosen, err := chooseWithTotalPower(g, a, controller, pid, raw)
			if err != nil {
				return err
			}
			all = append(all, chosen...)
			continue
		}
		if chosen, handled, err := chooseCardPowerLoops(g, a, controller, pid, pChoices, rawMin, minAmount, maxAmount); handled {
			if err != nil {
				return err
			}
			for _, cid := range chosen {
				if !slices.Contains(all, cid) {
					all = append(all, cid)
				}
			}
			continue
		}
		each, ok := a.Params.Param("ChooseEach")
		if hasParam(a, "EachBasicType") {
			// ChooseCardEffect.java:119-130, ahead of ChooseEach$: one land of
			// each basic type, in CardType.getBasicTypes()'s HashSet order,
			// each pick mandatory.
			each, ok = basicTypesHashOrder, true
		}
		if ok {
			chosen, err := chooseEachCards(g, a, controller, pid, pChoices, each, hasParam(a, "EachBasicType"))
			if err != nil {
				return err
			}
			for _, cid := range chosen {
				if !slices.Contains(all, cid) {
					all = append(all, cid)
				}
			}
			continue
		}
		if hasParam(a, "AtRandom") && len(pChoices) > 0 {
			// Aggregates.random's reservoir: Amount$ cards, the sample's order
			// (ChooseCardEffect.java:225-228, ahead of the default and quasi
			// search branches).
			for _, i := range g.randomSample(len(pChoices), maxAmount) {
				all = append(all, pChoices[i])
			}
			continue
		}
		if quasi {
			// ChooseCardEffect.java:260-265: one card, optional unless
			// Mandatory$; nothing chosen -- an empty pool, which Java's
			// chooseSingleEntityForEffect answers null without asking, or a
			// decline -- returns from the whole effect, so the host's chosen,
			// remembered and imprinted cards stay as they were.
			if len(pChoices) == 0 {
				return nil
			}
			lo := 0
			if _, mandatory := a.Params.Param("Mandatory"); mandatory {
				lo = 1
			}
			chosen := controller.ChooseCardsForEffect(g, pid, a.Source, pChoices, lo, 1)
			if err := checkChoice(chosen, pChoices, lo, 1); err != nil {
				return fmt.Errorf("engine: ChooseCard: %w", err)
			}
			if len(chosen) == 0 {
				return nil
			}
			all = append(all, chosen...)
			continue
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

	flushMap()
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

// basicTypesHashOrder is the iteration order of CardType.Constant.
// BASIC_TYPES, a HashSet<String>: buckets 4, 9, 9, 10, 13 for Mountain, Swamp,
// Forest, Plains, Island, the two in bucket 9 in the order TypeLists.txt
// adds them.
const basicTypesHashOrder = "Mountain & Swamp & Forest & Plains & Island"

// partyTypesHashOrder is the iteration order of CardType.Constant.PARTY_TYPES,
// Sets.newHashSet("Cleric", "Rogue", "Warrior", "Wizard"): Guava sizes the set
// for four elements (Maps.capacity(4) = 6, a table of 8), so the buckets are 2
// (Warrior), 5 (Rogue, then Wizard: insertion order) and 6 (Cleric). Computed
// from String.hashCode, not observed in a JVM (quirks.md).
const partyTypesHashOrder = "Warrior & Rogue & Wizard & Cleric"

// chooseEachCards is ChooseEach$ (ChooseCardEffect.java:131-146): for each
// " & "-separated type, in order, the chooser picks one of their cards of
// that type (optional unless Mandatory$; a type with none is skipped). One
// card can fill two types and is chosen once. "Party" walks Java's HashSet
// of the four party creature types (partyTypesHashOrder).
func chooseEachCards(g *Game, a *Ability, controller PlayerController, pid PlayerID, pool []CardID, each string, mandatoryPick bool) ([]CardID, error) {
	if each == "Party" {
		each = partyTypesHashOrder
	}
	lo := 0
	if mandatoryPick || hasParam(a, "Mandatory") {
		lo = 1
	}
	var chosen []CardID
	for _, kind := range strings.Split(each, " & ") {
		spec := valid.Parse(kind)
		var options []CardID
		for _, cid := range pool {
			if Matches(g, g.Card(cid), spec, a.Controller, a.Source) {
				options = append(options, cid)
			}
		}
		if len(options) == 0 {
			continue
		}
		pick := controller.ChooseCardsForEffect(g, pid, a.Source, options, lo, 1)
		if err := checkChoice(pick, options, lo, 1); err != nil {
			return nil, fmt.Errorf("engine: ChooseCard: %w", err)
		}
		for _, cid := range pick {
			if !slices.Contains(chosen, cid) {
				chosen = append(chosen, cid)
			}
		}
	}
	return chosen, nil
}

// chooseWithTotalPower is WithTotalPower$ (ChooseCardEffect.java:146-171): the
// chooser picks creatures they control one at a time, each pick leaving only
// creatures whose power fits the total less the picks' power, the negative
// powers still unpicked counted against it as an allowance. An empty pick asks
// whether to stop; a "no" asks again. The choice pool is the chooser's
// creatures, not Choices$.
func chooseWithTotalPower(g *Game, a *Ability, controller PlayerController, pid PlayerID, raw string) ([]CardID, error) {
	total, ok := resolveNamedAmount(g, a.Amounts, g.Card(a.Source), raw)
	if !ok {
		return nil, fmt.Errorf("engine: ChooseCard: WithTotalPower$ %q not resolvable yet", raw)
	}
	power := func(id CardID) (int, error) {
		n, ok := g.Card(id).Power()
		if !ok {
			return 0, fmt.Errorf("engine: ChooseCard: WithTotalPower$: a creature's power is not resolvable yet")
		}
		return n, nil
	}
	var negative []CardID
	negNum := 0
	for _, id := range g.creaturesInPlay(pid) {
		n, err := power(id)
		if err != nil {
			return nil, err
		}
		if n <= -1 {
			negative = append(negative, id)
			negNum += n
		}
	}
	fits := func(left int, taken []CardID) ([]CardID, error) {
		var out []CardID
		for _, id := range g.creaturesInPlay(pid) {
			n, err := power(id)
			if err != nil {
				return nil, err
			}
			if n <= left && !slices.Contains(taken, id) {
				out = append(out, id)
			}
		}
		return out, nil
	}
	creatures, err := fits(total-negNum, nil)
	if err != nil {
		return nil, err
	}
	var pool []CardID
	chosenPower := 0
	for len(creatures) > 0 {
		pick := controller.ChooseCardsForEffect(g, pid, a.Source, creatures, 0, 1)
		if err := checkChoice(pick, creatures, 0, 1); err != nil {
			return nil, fmt.Errorf("engine: ChooseCard: %w", err)
		}
		if len(pick) == 0 {
			if controller.ConfirmEffect(g, pid, a.Source) {
				break
			}
			continue
		}
		n, err := power(pick[0])
		if err != nil {
			return nil, err
		}
		chosenPower += n
		pool = append(pool, pick[0])
		negative = slices.DeleteFunc(negative, func(id CardID) bool { return id == pick[0] })
		negNum = 0
		for _, id := range negative {
			m, err := power(id)
			if err != nil {
				return nil, err
			}
			negNum += m
		}
		if creatures, err = fits(total-chosenPower-negNum, pool); err != nil {
			return nil, err
		}
	}
	return pool, nil
}

// chooseCardQuasiSearch reads QuasiLibrarySearch$ (ChooseCardEffect.java:
// 243-269): the choice is "search <player>'s library" -- Doubling Chant,
// Clarion Ultimatum, Dichotomancy pick one card from ChoiceZone$ Library --
// and reports whether the ability names it.
//
// The param's player is only the library Java reveals before the pick
// (DelayedReveal, :253-259), nothing this omniscient engine shows; an empty
// one is an error where Java's get(0) throws. Java checks neither
// Player.canSearchLibraryWith (Leonin Arbiter) nor LimitSearchLibrary (Aven
// Mindcensor: :254-256 shortens only the reveal, never the choices), so
// neither is read here (forge-java-defects.md).
//
// A chooser who is the activator and whose searches another player controls
// (ControlOpponentsSearchingLibrary$, ADR-0040) is refused: Java pushes the
// control grant at :245-251 and pops it at :267-269, after the `return` a
// declined or empty pick takes at :262-264, so a decline leaves the grant on
// for good. That is a Forge bug (PORT-8); reproducing it would leave another
// player controlling this one, so the shape is refused before anything is
// asked.
func chooseCardQuasiSearch(g *Game, a *Ability, choosers []PlayerID) (bool, error) {
	spec, ok := a.Params.Param("QuasiLibrarySearch")
	if !ok {
		return false, nil
	}
	searched, err := definedPlayers(g, a.Controller, a.Source, spec, a.refs())
	if err != nil {
		return false, fmt.Errorf("engine: ChooseCard: QuasiLibrarySearch$: %w", err)
	}
	if len(searched) == 0 {
		return false, fmt.Errorf("engine: ChooseCard: QuasiLibrarySearch$ %q names no player", spec)
	}
	for _, pid := range choosers {
		if who, _ := g.SearchController(pid); pid == a.Controller && who != NoPlayer {
			return false, fmt.Errorf("engine: ChooseCard: QuasiLibrarySearch$ under a ControlOpponentsSearchingLibrary$ redirect not resolvable yet (ChooseCardEffect.java:262-269 never ends the grant on a decline)")
		}
	}
	return true, nil
}

// chooseCardPool is the candidate list ChooseCardEffect.java builds before
// asking anyone: DefinedCards$ outright when present, otherwise every card
// in the ChoiceZone$ zones (comma-separated, default Battlefield) filtered
// by Choices$, in seat order per zone -- game.getCardsIn's own order.
func chooseCardPool(g *Game, a *Ability, source *Card) ([]CardID, error) {
	if defined, ok := a.Params.Param("DefinedCards"); ok {
		cards, err := definedCards(source, defined, a.refs())
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
	if _, ok := a.Params.Param("AllCards"); ok {
		// ChooseCardEffect.java:69: game.getCardsInGame() replaces the zones.
		for _, cid := range g.cardsInGame() {
			if !hasChoices || Matches(g, g.Card(cid), spec, a.Controller, a.Source) {
				pool = append(pool, cid)
			}
		}
		return pool, nil
	}
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

// chooseCardPowerLoops is ChooseCardEffect.java:171-224's three alternative
// selection loops, handled false for any other shape. WithDifferentPowers$
// asks for one creature at a time, optional once MinAmount$ are chosen, and
// drops every creature of a chosen power from the pool; EachDifferentPower$
// asks for one creature of each power in the pool, lowest power first;
// ControlAndNot$ takes Amount$ of the chooser's creatures and Amount$ of the
// others. The chosen collections are sets (FCollection), so a card picked
// twice stays once; Java's non-Mandatory pick is optional.
func chooseCardPowerLoops(g *Game, a *Ability, controller PlayerController, pid PlayerID, pool []CardID, rawMin, lo, hi int) ([]CardID, bool, error) {
	pick := func(from []CardID, least, most int) ([]CardID, error) {
		if most > len(from) {
			most = len(from)
		}
		if least > most {
			least = most
		}
		chosen := controller.ChooseCardsForEffect(g, pid, a.Source, from, least, most)
		if err := checkChoice(chosen, from, least, most); err != nil {
			return nil, fmt.Errorf("engine: ChooseCard: %w", err)
		}
		return chosen, nil
	}
	switch {
	case hasParam(a, "WithDifferentPowers"):
		var chosen []CardID
		for len(pool) > 0 && len(chosen) < hi {
			least := 1
			if len(chosen) >= rawMin {
				least = 0
			}
			got, err := pick(pool, least, 1)
			if err != nil {
				return nil, true, err
			}
			if len(got) == 0 {
				break
			}
			chosen = append(chosen, got[0])
			power := netPower(g.Card(got[0]))
			var rest []CardID
			for _, id := range pool {
				if netPower(g.Card(id)) != power {
					rest = append(rest, id)
				}
			}
			pool = rest
		}
		return chosen, true, nil
	case hasParam(a, "EachDifferentPower"):
		var powers []int
		for _, id := range pool {
			if p := netPower(g.Card(id)); !slices.Contains(powers, p) {
				powers = append(powers, p)
			}
		}
		slices.Sort(powers)
		var chosen []CardID
		for _, power := range powers {
			var valids []CardID
			for _, id := range pool {
				if netPower(g.Card(id)) == power {
					valids = append(valids, id)
				}
			}
			got, err := pick(valids, 1, 1)
			if err != nil {
				return nil, true, err
			}
			chosen = append(chosen, got...)
		}
		return chosen, true, nil
	case hasParam(a, "ControlAndNot"):
		var mine, theirs []CardID
		for _, id := range pool {
			if g.Card(id).Controller() == pid {
				mine = append(mine, id)
			} else {
				theirs = append(theirs, id)
			}
		}
		first, err := pick(mine, lo, hi)
		if err != nil {
			return nil, true, err
		}
		second, err := pick(theirs, lo, hi)
		if err != nil {
			return nil, true, err
		}
		return append(first, second...), true, nil
	}
	return nil, false, nil
}
