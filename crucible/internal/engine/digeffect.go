package engine

//enginelint:allow id card game player ability defined condition control amount parts zone effecthelpers zonemove changezonestatic

import (
	"fmt"
	"slices"
	"strings"

	"github.com/jczastkiewicz/crucible/internal/mana"
)

// digUnresolvedParams are DigEffect.java's params this port cannot honour
// yet: a non-library or bottom-up dig (SourceZone$, FromBottom$), the alternative selection loops
// (WithDifferentPowers$),
// entering-state modifiers (FaceDown*$, WithCounters$), exile
// bookkeeping, and GainControl$'s Cybership reorder.
var digUnresolvedParams = [...]string{
	"GainControl",
	"FaceDown", "FaceDownPower", "FaceDownToughness", "FaceDownSetType",
}

// digEffect is DigEffect.java: each Defined$/ValidTgts$ player (default
// You) looks at the top DigNum$ cards of their library; the activator
// chooses ChangeNum$ (default 1, or All, or Any) of those matching
// ChangeValid$ for DestinationZone$ (default Hand), and the rest go to
// DestinationZone2$ (default the library, LibraryPosition2$ default the
// bottom). Optional$ lets the chooser take none. Chosen cards are moved in
// reverse pick order (Java's own Collections.reverse), then re-ordered by
// their owners for a battlefield or library destination; the rest are
// ordered by the chooser (or by their owners into a graveyard) unless
// SkipReorder$ or RestRandomOrder$, which shuffles them with the game's own
// RNG. 813 real lines. Reveal$, NoReveal$, ForceReveal$, ForceRevealToController$,
// WithMayLook$ and NoLooking$ only decide what Java shows which player (a
// reveal event, a DelayedReveal view, a look-at grant on the view); the
// engine is omniscient, so they are accepted and read nothing (quirks.md).
type digEffect struct{}

func (digEffect) Resolve(g *Game, a *Ability, controller PlayerController) error {
	if err := rejectUnreadCondition(a, a.Params.Name); err != nil {
		return err
	}
	for _, key := range digUnresolvedParams {
		if _, ok := a.Params.Param(key); ok {
			return fmt.Errorf("engine: Dig: %s$ not resolvable yet", key)
		}
	}
	source := g.Card(a.Source)
	if !subAbilityConditionMet(g, source, a.Amounts, a.Params) {
		return nil
	}
	// SpellAbilityEffect.java:1113-1114: the first DefinedExiler$ player, else
	// the activator (markExiledWith's default).
	exiler, hasExiler := NoPlayer, false
	if raw, ok := a.Params.Param("DefinedExiler"); ok {
		ps, err := definedPlayers(g, a.Controller, a.Source, raw, a.refs())
		if err != nil {
			return fmt.Errorf("engine: Dig: %w", err)
		}
		if len(ps) == 0 {
			return fmt.Errorf("engine: Dig: DefinedExiler$ %q names no player", raw)
		}
		exiler, hasExiler = ps[0], true
	}
	digRaw, _ := a.Params.Param("DigNum")
	digNum, ok := resolveNamedAmount(g, a.Amounts, source, digRaw)
	if !ok {
		return fmt.Errorf("engine: Dig: DigNum$ %q not resolvable yet", digRaw)
	}
	dest1, err := digZoneParam(a, "DestinationZone", Hand)
	if err != nil {
		return err
	}
	srcZone, err := digZoneParam(a, "SourceZone", Library)
	if err != nil {
		return err
	}
	dest2, err := digZoneParam(a, "DestinationZone2", Library)
	if err != nil {
		return err
	}
	libPos1, err := digLibraryPos(a, "LibraryPosition")
	if err != nil {
		return err
	}
	libPos2, err := digLibraryPos(a, "LibraryPosition2")
	if err != nil {
		return err
	}
	changeValid, _ := a.Params.Param("ChangeValid")
	changeNum, changeAll, anyNumber := 1, false, false
	if raw, ok := a.Params.Param("ChangeNum"); ok {
		switch {
		case strings.EqualFold(raw, "All"):
			changeAll = true
		case strings.EqualFold(raw, "Any"):
			anyNumber = true
		default:
			n, ok := resolveNamedAmount(g, a.Amounts, source, raw)
			if !ok {
				return fmt.Errorf("engine: Dig: ChangeNum$ %q not resolvable yet", raw)
			}
			changeNum = n
		}
	}
	_, optional := a.Params.Param("Optional")
	_, mayBeSkipped := a.Params.Param("PromptToSkipOptionalAbility")
	_, remZone1 := a.Params.Param("RememberChanged")
	remZone2 := false
	if raw, ok := a.Params.Param("RememberMovedToZone"); ok {
		remZone1 = remZone1 || strings.Contains(raw, "1")
		remZone2 = strings.Contains(raw, "2")
	}
	_, tapped := a.Params.Param("Tapped")

	players, err := targetedOrDefinedPlayers(g, a.Controller, a.Source, a.Params, a.refs())
	if err != nil {
		return fmt.Errorf("engine: Dig: %w", err)
	}
	var moved1, moved2 []CardID
	// StaticEffect$ is applied by the moves themselves (GameAction.setupStaticEffect,
	// :679-730); WithCounters$ puts counters on a card entering the battlefield
	// (DigEffect.java:389-405), the activator placing them.
	staticTmpl, err := changeZoneStaticTemplate(g, a)
	if err != nil {
		return err
	}
	var enter []enterCounters
	if kind, ok := a.Params.Param("WithCounters"); ok {
		n := 1
		if raw, ok := a.Params.Param("WithCountersAmount"); ok {
			if n, ok = resolveNamedAmount(g, a.Amounts, source, raw); !ok {
				return fmt.Errorf("engine: Dig: WithCountersAmount$ %q not resolvable yet", raw)
			}
		}
		if dest1 == Battlefield {
			enter = []enterCounters{{kind: CounterType(strings.ToUpper(strings.TrimSpace(kind))), n: n, placer: a.Controller}}
		}
	}
	// ExileWithCounters$ (DigEffect.java:414, :472): one counter of the named
	// kind on each card exiled, placed by the activator through the dig's
	// counter table, which is flushed once every card has moved (:490).
	exileCounter, hasExileCounter := a.Params.Param("ExileWithCounters")
	var counterTable []CardID
	chooser := a.Controller
	for _, p := range players {
		if g.Player(p).Lost {
			continue
		}
		library := g.Zone(srcZone, p).Cards()
		n := digNum
		if n > len(library) {
			n = len(library)
		}
		top := append([]CardID(nil), library[:n]...)
		if _, ok := a.Params.Param("FromBottom"); ok {
			// DigEffect.java:181-183 reverses the zone list before taking
			// the first DigNum$, so the bottom card is looked at first.
			top = top[:0]
			for i := len(library) - 1; i >= len(library)-n; i-- {
				top = append(top, library[i])
			}
		}
		if len(top) == 0 {
			continue
		}
		hasRevealed := true
		if _, ok := a.Params.Param("RevealOptional"); ok {
			hasRevealed = controller.ConfirmEffect(g, p, a.Source)
		}
		if _, ok := a.Params.Param("RememberRevealed"); ok && hasRevealed {
			for _, id := range top {
				source.Memory.Remember(CardEntity(id))
			}
		}
		if _, ok := a.Params.Param("ImprintRevealed"); ok && hasRevealed {
			for _, id := range top {
				source.Memory.Imprint(id)
			}
		}
		if raw, ok := a.Params.Param("Choser"); ok {
			// DigEffect.java:215-222: the activator picks the chooser among
			// the players Choser$ names; SetChosenPlayer$ records the pick.
			choosers, err := definedPlayers(g, a.Controller, a.Source, raw, a.refs())
			if err != nil {
				return fmt.Errorf("engine: Dig: Choser$: %w", err)
			}
			if len(choosers) > 0 {
				chooser = choosers[0]
				if len(choosers) > 1 {
					chooser = controller.ChoosePlayerForEffect(g, a.Controller, a.Source, choosers)
				}
			}
			if _, ok := a.Params.Param("SetChosenPlayer"); ok {
				source.Memory.SetChosenPlayer(chooser)
			}
		}
		valid := top
		totalCMC := hasParam(a, "WithTotalCMC")
		budget := 0
		if totalCMC {
			// DigEffect.java:154-155, :227-229: the budget prefilters before
			// ChangeValid$ and shrinks with every pick.
			if budget, err = optionalAmount(g, a, "Dig", "WithTotalCMC", 0); err != nil {
				return err
			}
			valid = filterValid(g, valid, fmt.Sprintf("Card.cmcLE%d", budget), source.Controller(), a.Source)
		}
		if changeValid != "" {
			valid = filterValid(g, valid, changeValid, source.Controller(), a.Source)
		}
		if optional && mayBeSkipped && len(valid) > 0 && !controller.ConfirmEffect(g, p, a.Source) {
			return nil
		}
		var chosen []CardID
		switch {
		case changeAll:
			chosen = append(chosen, valid...)
		case hasParam(a, "RandomChange"):
			// DigEffect.java:260-262, CardLists.getRandomSubList (:150-161): no
			// decision; each pick shuffles what is left and takes the first.
			rest := slices.Clone(valid)
			for n := min(changeNum, len(valid)); n > 0; n-- {
				g.rand.Shuffle(len(rest), func(i, j int) { rest[i], rest[j] = rest[j], rest[i] })
				chosen = append(chosen, rest[0])
				rest = rest[1:]
			}
		case totalCMC:
			// DigEffect.java:263-289: one pick at a time while the budget
			// lasts; optional picks may stop early.
			for len(valid) > 0 && (anyNumber || len(chosen) < changeNum) {
				lo := 1
				if anyNumber || optional {
					lo = 0
				}
				pick := controller.ChooseCardsForEffect(g, chooser, a.Source, valid, lo, 1)
				if err := checkChoice(pick, valid, lo, 1); err != nil {
					return fmt.Errorf("engine: Dig: %w", err)
				}
				if len(pick) == 0 {
					break
				}
				chosen = append(chosen, pick[0])
				valid = withoutCards(valid, pick)
				budget -= g.Card(pick[0]).CMC()
				valid = filterValid(g, valid, fmt.Sprintf("Card.cmcLE%d", budget), source.Controller(), a.Source)
			}
		case hasParam(a, "WithDifferentPowers"):
			// DigEffect.java:307-316: one optional pick at a time; each pick
			// drops every card of its power. Unlike ChooseCard's loop, a pick
			// is added once.
			for len(valid) > 0 && (anyNumber || len(chosen) < changeNum) {
				pick := controller.ChooseCardsForEffect(g, chooser, a.Source, valid, 0, 1)
				if err := checkChoice(pick, valid, 0, 1); err != nil {
					return fmt.Errorf("engine: Dig: %w", err)
				}
				if len(pick) == 0 {
					break
				}
				chosen = append(chosen, pick[0])
				power := netPower(g.Card(pick[0]))
				var rest []CardID
				for _, id := range valid {
					if netPower(g.Card(id)) != power {
						rest = append(rest, id)
					}
				}
				valid = rest
			}
		case hasParam(a, "ForEachColorPair"):
			// DigEffect.java:290-306: one card per two-color pair, in
			// MagicColor.COLORPAIR order (WU UB BR RG GW WB UR BG RW GU), each
			// exactly those two colors (Niv-Mizzet Reborn); a pair with no
			// match is skipped without a question, ChangeNum$ is not read.
			for _, pair := range [...]mana.Colors{
				mana.White | mana.Blue, mana.Blue | mana.Black, mana.Black | mana.Red,
				mana.Red | mana.Green, mana.Green | mana.White, mana.White | mana.Black,
				mana.Blue | mana.Red, mana.Black | mana.Green, mana.Red | mana.White,
				mana.Green | mana.Blue} {
				var list []CardID
				for _, id := range valid {
					if g.Card(id).Colors() == pair {
						list = append(list, id)
					}
				}
				if len(list) == 0 {
					continue
				}
				pick := controller.ChooseCardsForEffect(g, chooser, a.Source, list, 1, 1)
				if err := checkChoice(pick, list, 1, 1); err != nil {
					return fmt.Errorf("engine: Dig: %w", err)
				}
				chosen = append(chosen, pick...)
			}
		case len(valid) > 0:
			hi := changeNum
			if anyNumber || hi > len(valid) {
				hi = len(valid)
			}
			lo := hi
			if anyNumber || optional {
				lo = 0
			}
			if hi > 0 {
				chosen = controller.ChooseCardsForEffect(g, chooser, a.Source, valid, lo, hi)
				if err := checkChoice(chosen, valid, lo, hi); err != nil {
					return fmt.Errorf("engine: Dig: %w", err)
				}
			}
		}
		if _, ok := a.Params.Param("ForgetOtherRemembered"); ok {
			source.Memory.ClearRemembered()
		}
		chosen = reversedCards(chosen)
		if dest1 == Battlefield || dest1 == Library {
			chosen, err = g.orderCardsByTheirOwners(controller, chosen, dest1)
			if err != nil {
				return fmt.Errorf("engine: Dig: %w", err)
			}
		}
		for _, id := range chosen {
			// DigEffect.java:409: Attacking$/Blocking$ join combat as the card
			// enters, changeZoneJoinCombat's own timing note.
			joined, err := g.changeZoneJoinCombat(controller, a, id, dest1, NoPlayer)
			if err != nil {
				return fmt.Errorf("engine: Dig: %w", err)
			}
			melded := g.moveByEffect(controller, id, dest1, libPos1, NoPlayer, tapped, enter...)
			g.changeZoneLeftCombat(id, joined)
			g.applyChangeZoneStatic(staticTmpl, id)
			if dest1 == Exile {
				g.markExiledWith(id, a.Source)
				if hasExiler {
					g.markExiledBy(id, exiler)
				}
				if hasExileCounter {
					counterTable = append(counterTable, id)
				}
				g.exileFaceDown(a, id)
				if melded != NoCard {
					g.markExiledWith(melded, a.Source)
				}
			}
			moved1 = append(moved1, id)
			if _, ok := a.Params.Param("Imprint"); ok {
				source.Memory.Imprint(id)
			}
			if remZone1 {
				source.Memory.Remember(CardEntity(id))
			}
		}
		rest := withoutCards(top, chosen)
		if len(rest) == 0 {
			continue
		}
		if _, ok := a.Params.Param("DestZone2Optional"); ok && !controller.ConfirmEffect(g, p, a.Source) {
			continue
		}
		if dest2 == Library || dest2 == Graveyard {
			_, random := a.Params.Param("RestRandomOrder")
			_, skip := a.Params.Param("SkipReorder")
			switch {
			case random:
				g.rand.Shuffle(len(rest), func(i, j int) { rest[i], rest[j] = rest[j], rest[i] })
			case !skip && len(rest) > 1 && dest2 == Graveyard:
				rest, err = g.orderCardsByTheirOwners(controller, rest, dest2)
				if err != nil {
					return fmt.Errorf("engine: Dig: %w", err)
				}
			case !skip && len(rest) > 1:
				ordered := controller.OrderCardsForZone(g, chooser, rest, dest2)
				if err := checkChoice(ordered, rest, len(rest), len(rest)); err != nil {
					return fmt.Errorf("engine: Dig: %w", err)
				}
				rest = ordered
			}
		}
		for _, id := range rest {
			melded := g.moveByEffect(controller, id, dest2, libPos2, NoPlayer, false)
			g.applyChangeZoneStatic(staticTmpl, id)
			if dest2 == Exile {
				g.markExiledWith(id, a.Source)
				if hasExiler {
					g.markExiledBy(id, exiler)
				}
				if hasExileCounter {
					counterTable = append(counterTable, id)
				}
				if melded != NoCard {
					g.markExiledWith(melded, a.Source)
				}
			}
			moved2 = append(moved2, id)
			if remZone2 {
				source.Memory.Remember(CardEntity(id))
			}
		}
	}
	g.checkChangesZoneAllTriggers(controller, moved1, srcZone, dest1)
	g.checkChangesZoneAllTriggers(controller, moved2, srcZone, dest2)
	for _, id := range counterTable {
		g.addCardCounters(controller, a.Controller, a.Source, id, CounterType(strings.ToUpper(exileCounter)), 1)
	}
	return nil
}

func digZoneParam(a *Ability, key string, def ZoneType) (ZoneType, error) {
	raw, ok := a.Params.Param(key)
	if !ok {
		return def, nil
	}
	z, err := changeZoneDestination(raw)
	if err != nil {
		return 0, fmt.Errorf("engine: Dig: %s$: %w", key, err)
	}
	return z, nil
}

// digLibraryPos reads a literal LibraryPosition$/LibraryPosition2$ (Java's
// Integer.parseInt), default the bottom; only top and bottom are movable.
func digLibraryPos(a *Ability, key string) (int, error) {
	raw, ok := a.Params.Param(key)
	if !ok {
		return libraryBottom, nil
	}
	switch strings.TrimSpace(raw) {
	case "0":
		return 0, nil
	case "-1":
		return libraryBottom, nil
	}
	return 0, fmt.Errorf("engine: Dig: %s$ %q not resolvable yet", key, raw)
}
