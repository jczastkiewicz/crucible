package engine

//enginelint:allow id card game ability defined condition control parts zone effecthelpers valid zonemove damageresolveeffect player untilcommand controlcommands

import (
	"fmt"
	"slices"
	"strings"

	"github.com/jczastkiewicz/crucible/internal/cardtype"
	"github.com/jczastkiewicz/crucible/internal/valid"
)

// repeatEachUnresolvedParams are RepeatEachEffect.java's params this port
// cannot honour yet: the stack-object loop
// (RepeatSpellAbilities$), a delayed per-player loop
// (NextTurnForEachPlayer$), the batched
// life table (LoseLifeMap$). ChooseOrder$ orders a card loop and names
// RepeatTypesFrom$'s chooser; the player loop, which Java also reads it
// for, stays as it is.
var repeatEachUnresolvedParams = [...]string{
	"RepeatSpellAbilities",
	"LoseLifeMap",
	"Ultimate",
}

// repeatEachEffect is RepeatEachEffect.java: RepeatSubAbility$ resolves
// once per item, the item swapped into the host's Memory for the duration
// so the sub-ability reads it as Remembered (or Imprinted, UseImprinted$).
// Items are RepeatCards$ (cards in Zone$, default Battlefield, matching it,
// relative to the host's controller), DefinedCards$, RepeatTargeted$ (the
// ability's own targets) and RepeatPlayers$ (a Defined$ player list; the
// host's other remembered players are set aside while each resolves, and
// RepeatOptionalForEachPlayer$ lets each player decline). 288 real lines.
type repeatEachEffect struct{}

func (repeatEachEffect) Resolve(g *Game, a *Ability, controller PlayerController) error {
	if err := rejectUnreadCondition(a, a.Params.Name); err != nil {
		return err
	}
	for _, key := range repeatEachUnresolvedParams {
		if _, ok := a.Params.Param(key); ok {
			return fmt.Errorf("engine: RepeatEach: %s$ not resolvable yet", key)
		}
	}
	source := g.Card(a.Source)
	if !subAbilityConditionMet(g, source, a.Amounts, a.Params) {
		return nil
	}
	if _, ok := a.Params.Param("Optional"); ok {
		if _, prompt := a.Params.Param("OptionPrompt"); prompt && !controller.ConfirmEffect(g, a.Controller, a.Source) {
			return nil
		}
	}
	if _, ok := a.Params.Param("ChangeZoneTable"); ok {
		// RepeatEachEffect.java:84-86, :202-205: one CardZoneTable for the whole
		// loop; the batched ChangesZoneAll fires when it ends.
		defer g.openZoneBatch(controller)()
	}
	if hasParam(a, "DamageMap") {
		// RepeatEachEffect.java:79-82: every DealDamage in the loop records into
		// this table (subability.go hands it down); it is dealt once after the
		// loop with this ability as the cause.
		a.damageMap = &pendingDamage{}
	}
	_, useImprinted := a.Params.Param("UseImprinted")
	m := &source.Memory

	var cards []CardID
	hasCards := false
	if spec, ok := a.Params.Param("RepeatCards"); ok {
		hasCards = true
		zones := []ZoneType{Battlefield}
		if raw, ok := a.Params.Param("Zone"); ok {
			var err error
			zones, err = parseZoneList(raw)
			if err != nil {
				return fmt.Errorf("engine: RepeatEach: Zone$: %w", err)
			}
		}
		parsed := valid.Parse(spec)
		for _, z := range zones {
			for _, pid := range g.Players() {
				for _, id := range g.Zone(z, pid).Cards() {
					if Matches(g, g.Card(id), parsed, source.Controller(), a.Source) {
						cards = append(cards, id)
					}
				}
			}
		}
	} else if spec, ok := a.Params.Param("DefinedCards"); ok {
		hasCards = true
		var err error
		cards, err = definedCards(source, spec, a.refs())
		if err != nil {
			return fmt.Errorf("engine: RepeatEach: DefinedCards$: %w", err)
		}
	}
	if _, ok := a.Params.Param("ClearRemembered"); ok {
		m.ClearRemembered()
	}
	if order, ok := a.Params.Param("ChooseOrder"); ok && len(cards) > 1 {
		// RepeatEachEffect.java:92-96: the chooser (the activator for "True",
		// else the first Defined$ player) orders the cards.
		chooser := a.Controller
		if order != "True" {
			ps, err := definedPlayers(g, a.Controller, a.Source, order, a.refs())
			if err != nil {
				return fmt.Errorf("engine: RepeatEach: ChooseOrder$: %w", err)
			}
			if len(ps) == 0 {
				return fmt.Errorf("engine: RepeatEach: ChooseOrder$ %q names no player", order)
			}
			chooser = ps[0]
		}
		ordered := controller.OrderCardsForZone(g, chooser, slices.Clone(cards), None)
		if err := checkChoice(ordered, cards, len(cards), len(cards)); err != nil {
			return fmt.Errorf("engine: RepeatEach: ordering the cards: %w", err)
		}
		cards = ordered
	}
	if hasCards {
		for _, id := range cards {
			if useImprinted {
				m.Imprint(id)
			} else {
				m.Remember(CardEntity(id))
			}
			a.setVoteAmount(CardEntity(id))
			if err := g.resolveAdditionalKey(a, controller, "RepeatSubAbility"); err != nil {
				return err
			}
			if useImprinted {
				m.forgetImprinted(id)
			} else {
				m.Forget(CardEntity(id))
			}
		}
	}
	if _, ok := a.Params.Param("RepeatTargeted"); ok {
		for _, e := range a.Targets {
			m.Remember(e)
			if err := g.resolveAdditionalKey(a, controller, "RepeatSubAbility"); err != nil {
				return err
			}
			m.Forget(e)
		}
	}
	if def, ok := a.Params.Param("RepeatTypesFrom"); ok {
		if err := repeatCardTypes(g, a, controller, def); err != nil {
			return err
		}
	}
	if spec, ok := a.Params.Param("RepeatPlayers"); ok {
		players, err := definedPlayers(g, a.Controller, a.Source, spec, a.refs())
		if err != nil {
			return fmt.Errorf("engine: RepeatEach: RepeatPlayers$: %w", err)
		}
		if start, ok := a.Params.Param("StartingWith"); ok {
			if players, err = playersStartingWith(g, a, players, start); err != nil {
				return err
			}
		} else {
			// getPlayers sorts by turn order from the active player
			// (SpellAbilityEffect.java:351-357).
			players = apnapSubset(g, players)
		}
		if _, ok := a.Params.Param("ClearRememberedBeforeLoop"); ok {
			m.ClearRemembered()
		}
		_, optional := a.Params.Param("RepeatOptionalForEachPlayer")
		_, nextTurn := a.Params.Param("NextTurnForEachPlayer")
		for _, p := range players {
			if optional && !controller.ConfirmEffect(g, p, a.Source) {
				continue
			}
			if nextTurn {
				// :169-183: the loop for p waits for p's next turn (the confirm
				// above is asked now); no vote amount is set on this path.
				g.timestamp++
				g.pendingRepeats = append(g.pendingRepeats, pendingRepeat{ID: g.timestamp, Ability: *a, Player: p})
				g.phaseCmds = append(g.phaseCmds, phaseCommand{
					cardCommand: cardCommand{Kind: commandRepeatForPlayer, Timestamp: g.timestamp, Host: a.Source},
					When:        untilTurnStart, Player: p,
				})
				continue
			}
			old := swapRememberedPlayer(m, p)
			a.setVoteAmount(PlayerEntity(p))
			if err := g.resolveAdditionalKey(a, controller, "RepeatSubAbility"); err != nil {
				return err
			}
			restoreRememberedPlayers(m, p, old)
		}
	}
	if a.damageMap != nil && hasParam(a, "DamageMap") {
		// RepeatEachEffect.java:199-201.
		entries := a.damageMap.entries
		a.damageMap.entries = nil
		table := g.dealPendingDamage(controller, entries)
		if hasParam(a, "RememberDamaged") {
			rememberDamaged(g, a.Source, table)
		}
		g.checkDamageTableTriggers(controller, table, false)
	}
	return nil
}

// playersStartingWith is getPlayers' trailing sort (SpellAbilityEffect.java:
// 353-358): players in turn order from the first player StartingWith$ names
// (StartingWith$ You is the ability's controller). The default start, the
// active player, is apnapSubset.
func playersStartingWith(g *Game, a *Ability, players []PlayerID, spec string) ([]PlayerID, error) {
	starters, err := definedPlayers(g, a.Controller, a.Source, spec, a.refs())
	if err != nil {
		return nil, fmt.Errorf("engine: RepeatEach: StartingWith$: %w", err)
	}
	if len(starters) == 0 {
		return players, nil
	}
	order := g.playersInTurnOrder()
	from := slices.Index(order, starters[0])
	out := make([]PlayerID, 0, len(players))
	for i := range order {
		if id := order[(from+i)%len(order)]; slices.Contains(players, id) {
			out = append(out, id)
		}
	}
	return out, nil
}

// repeatCardTypes is RepeatEachEffect.java:132-161: the core types among the
// cards def names (ThisTurnCast_<valid>: the spells cast this turn that match,
// else a Defined$ card list) are chosen one by one -- the chooser is the
// activator, or ChooseOrder$'s first player -- each becoming the host's chosen
// type while RepeatSubAbility$ resolves. Java offers a HashSet of type names;
// this port offers them in the order they were first met, which only a
// controller that takes the first option can tell apart.
func repeatCardTypes(g *Game, a *Ability, controller PlayerController, def string) error {
	source := g.Card(a.Source)
	var types []string
	add := func(line cardtype.Line) {
		for _, t := range line.CoreTypes() {
			if !slices.Contains(types, t.String()) {
				types = append(types, t.String())
			}
		}
	}
	if rest, ok := strings.CutPrefix(def, "ThisTurnCast"); ok {
		_, spec, _ := strings.Cut(rest, "_")
		for _, line := range g.thisTurnCastTypes(a.Source, spec) {
			add(line)
		}
	} else {
		cards, err := definedCards(source, def, a.refs())
		if err != nil {
			return fmt.Errorf("engine: RepeatEach: RepeatTypesFrom$: %w", err)
		}
		for _, id := range cards {
			add(g.Card(id).Type())
		}
	}
	chooser := a.Controller
	if order, ok := a.Params.Param("ChooseOrder"); ok && order != "True" {
		ps, err := definedPlayers(g, a.Controller, a.Source, order, a.refs())
		if err != nil {
			return fmt.Errorf("engine: RepeatEach: ChooseOrder$: %w", err)
		}
		if len(ps) == 0 {
			return fmt.Errorf("engine: RepeatEach: ChooseOrder$ %q names no player", order)
		}
		chooser = ps[0]
	}
	stored := source.Memory.ChosenType(false)
	for len(types) > 0 {
		i := controller.ChooseOption(g, chooser, a.Source, types)
		if i < 0 || i >= len(types) {
			return fmt.Errorf("engine: RepeatEach: RepeatTypesFrom$: choice %d out of range", i)
		}
		source.Memory.SetChosenType(types[i], false)
		if err := g.resolveAdditionalKey(a, controller, "RepeatSubAbility"); err != nil {
			return err
		}
		types = slices.Delete(types, i, i+1)
	}
	source.Memory.SetChosenType(stored, false)
	return nil
}

// setVoteAmount is AmountFromVotes$: the Votes SVar of this ability becomes
// the votes e received (RepeatEachEffect.java:216-229), and stays for the
// sub-abilities that follow, as Java's setSVar leaves it.
func (a *Ability) setVoteAmount(e EntityID) {
	if hasParam(a, "AmountFromVotes") {
		a.Amounts = withAmount(a.Amounts, "Votes", a.votesFor(e))
	}
}

// apnapSubset is players in turn order from the active player.
func apnapSubset(g *Game, players []PlayerID) []PlayerID {
	out := make([]PlayerID, 0, len(players))
	for _, id := range g.playersInAPNAPOrder() {
		if slices.Contains(players, id) {
			out = append(out, id)
		}
	}
	return out
}

// pendingRepeat is a NextTurnForEachPlayer$ loop for one player: a copy of
// the RepeatEach ability, resolved when Player's next turn starts.

// runPendingRepeat is the closure RepeatEachEffect.java:176-183 registers:
// the host's other remembered players are set aside, p is remembered, the
// RepeatSubAbility resolves, and the set-aside players are restored. An error
// is left pending on the game (the command has no error path, GO-7).
func (g *Game) runPendingRepeat(controller PlayerController, r pendingRepeat) {
	m := &g.Card(r.Ability.Source).Memory
	old := swapRememberedPlayer(m, r.Player)
	a := r.Ability
	if err := g.resolveAdditionalKey(&a, controller, "RepeatSubAbility"); err != nil {
		g.recordPendingError(err)
	}
	restoreRememberedPlayers(m, r.Player, old)
}
