package engine

//enginelint:allow control game ability effecthelpers card condition defined parts choosecardeffect amount zone zonemove

import (
	"fmt"
	"slices"
	"strings"

	"github.com/jczastkiewicz/crucible/internal/cardtype"
)

// chooseTypeEffect is ChooseTypeEffect.java: each target player (the
// activator by default) chooses a type from Type$'s list -- every core type
// for Card, every creature/basic land/nonbasic land/land/planeswalker type
// for the others -- or from ValidTypes$ when given, less InvalidTypes$. The
// host records it (setChosenType, or setChosenType2 with ChooseType2$).
// AtRandom$ picks with Aggregates.random, only over an explicit ValidTypes$
// list: Java's subtype lists are HashSet iterations (CardType.Constant,
// CardType.java:871-873) whose order this port does not reproduce. Subtype
// lists come from the game DB's type vocabulary and are offered sorted.
// Secretly$ writes the same chosen-type slot without updating the view
// (Card.setSecretChosenType, Card.java:2214-2216).
type chooseTypeEffect struct{}

func (chooseTypeEffect) Resolve(g *Game, a *Ability, controller PlayerController) error {
	if err := rejectUnreadCondition(a, a.Params.Name); err != nil {
		return err
	}
	if err := rejectParams(a, "ChooseType"); err != nil {
		return err
	}
	source := g.Card(a.Source)
	if !subAbilityConditionMet(g, source, a.Amounts, a.Params) {
		return nil
	}
	kind, _ := a.Params.Param("Type")
	var types []string
	_, fromDefined := a.Params.Param("TypesFromDefined")
	if raw, ok := a.Params.Param("ValidTypes"); ok {
		types = strings.Split(raw, ",")
	} else if fromDefined {
		var err error
		if types, err = chooseTypesFromDefined(g, a, kind); err != nil {
			return err
		}
	} else if raw, ok := a.Params.Param("MostPrevalentInDefinedZone"); ok && kind == "Creature" {
		var err error
		if types, err = g.mostPrevalentCreatureTypes(a, raw); err != nil {
			return err
		}
	} else if kind == "CreatureInTargetedDeck" {
		var err error
		if types, err = g.creatureTypesInTargetedDeck(a); err != nil {
			return err
		}
	} else {
		if hasParam(a, "AtRandom") && kind != "Basic Land" {
			// Creature and Land lists are CardType's HashSets (CREATURE_TYPES,
			// LAND_TYPES) whose bucket order this port does not reproduce;
			// Basic Land's five-name order is known (basicTypesHashOrder).
			return fmt.Errorf("engine: ChooseType: AtRandom$ over Type$ %q not resolvable (no stable order)", kind)
		}
		var err error
		types, err = chooseTypeOptions(g, kind)
		if err != nil {
			return err
		}
	}
	if raw, ok := a.Params.Param("InvalidTypes"); ok {
		drop := strings.Split(raw, ",")
		kept := types[:0:0]
		for _, t := range types {
			if !containsString(drop, t) {
				kept = append(kept, t)
			}
		}
		types = kept
	}
	if hasParam(a, "Note") {
		// :117-121: each already noted type leaves the list once, before the
		// players choose, so two choosers may pick the same one (PORT-7).
		for _, noted := range source.Memory.NotedTypes() {
			if i := slices.Index(types, noted); i >= 0 {
				types = slices.Delete(slices.Clone(types), i, i+1)
			}
		}
	}
	if len(types) == 0 {
		if _, prevalent := a.Params.Param("MostPrevalentInDefinedZone"); fromDefined || prevalent {
			// :123-124: nothing to choose from is a no-op (a zone with no
			// creature has no most prevalent type, Favored Enemy's empty library).
			return nil
		}
		return fmt.Errorf("engine: ChooseType: no types to choose from")
	}
	players, err := targetedOrDefinedPlayers(g, a.Controller, a.Source, a.Params, a.refs())
	if err != nil {
		return fmt.Errorf("engine: ChooseType: %w", err)
	}
	_, second := a.Params.Param("ChooseType2")
	for _, p := range players {
		var i int
		if hasParam(a, "AtRandom") {
			i = g.randomIndex(len(types))
		} else {
			i = controller.ChooseOption(g, p, a.Source, types)
			if i < 0 || i >= len(types) {
				return fmt.Errorf("engine: ChooseType: choice %d out of range", i)
			}
		}
		if hasParam(a, "Note") {
			source.Memory.AddNotedType(types[i])
			if !hasParam(a, "ChooseNoted") {
				// :140-142: a noted type is not the chosen type unless asked.
				continue
			}
		}
		source.Memory.SetChosenType(types[i], second)
	}
	return nil
}

// chooseTypesFromDefined is ChooseTypeEffect.java:52-55 and :94-96: the
// creature types of the Defined$ cards, concatenated with duplicates kept, or
// for Type$ Shared the core types every defined card has (two cards at least).
func chooseTypesFromDefined(g *Game, a *Ability, kind string) ([]string, error) {
	spec, _ := a.Params.Param("TypesFromDefined")
	cards, err := definedCards(g.Card(a.Source), spec, a.refs())
	if err != nil {
		return nil, fmt.Errorf("engine: ChooseType: TypesFromDefined$: %w", err)
	}
	switch kind {
	case "Creature":
		reg := g.db.Types()
		if reg == nil {
			return nil, fmt.Errorf("engine: ChooseType: TypesFromDefined$ needs the type vocabulary")
		}
		var out []string
		for _, id := range cards {
			out = append(out, creatureTypesOf(g.Card(id).Type(), reg)...)
		}
		return out, nil
	case "Shared":
		if len(cards) < 2 {
			return nil, nil
		}
		var out []string
		for t := cardtype.Kindred; t <= cardtype.Vanguard; t++ {
			shared := true
			for _, id := range cards {
				if !g.Card(id).Type().HasStringType(t.String()) {
					shared = false
					break
				}
			}
			if shared {
				out = append(out, t.String())
			}
		}
		return out, nil
	}
	return nil, fmt.Errorf("engine: ChooseType: TypesFromDefined$ with Type$ %q not resolvable yet", kind)
}

// mostPrevalentCreatureTypes is MostPrevalentInDefinedZone$ <Defined>_<Zone>
// (ChooseTypeEffect.java:56-63, CardFactoryUtil.getMostProminentCreatureType):
// the creature types held by the most cards of the first defined player's
// zone, every tied one, sorted. No such type leaves the list empty.
func (g *Game) mostPrevalentCreatureTypes(a *Ability, raw string) ([]string, error) {
	who, zoneName, _ := strings.Cut(raw, "_")
	players, err := definedPlayers(g, a.Controller, a.Source, who, a.refs())
	if err != nil {
		return nil, fmt.Errorf("engine: ChooseType: MostPrevalentInDefinedZone$: %w", err)
	}
	if len(players) == 0 {
		// :58 takes get(0) of an empty list: a crash in Java (PORT-8).
		return nil, fmt.Errorf("engine: ChooseType: MostPrevalentInDefinedZone$ %q names no player", who)
	}
	zone := Battlefield
	if zoneName != "" {
		zones, err := parseZoneList(zoneName)
		if err != nil || len(zones) != 1 {
			return nil, fmt.Errorf("engine: ChooseType: MostPrevalentInDefinedZone$ zone %q", zoneName)
		}
		zone = zones[0]
	}
	reg := g.db.Types()
	if reg == nil {
		return nil, fmt.Errorf("engine: ChooseType: MostPrevalentInDefinedZone$ needs the type vocabulary")
	}
	counts := map[string]int{}
	most := 0
	for _, id := range g.Zone(zone, players[0]).Cards() {
		for _, t := range creatureTypesOf(g.Card(id).Type(), reg) {
			counts[t]++
			most = max(most, counts[t])
		}
	}
	var out []string
	for t, n := range counts {
		if n == most && most > 0 {
			out = append(out, t)
		}
	}
	slices.Sort(out)
	return out, nil
}

// chooseTypeOptions is ChooseTypeEffect's Type$ switch for the lists it
// builds itself.
func chooseTypeOptions(g *Game, kind string) ([]string, error) {
	if kind == "Card" {
		out := make([]string, 0, 15)
		for t := cardtype.Kindred; t <= cardtype.Vanguard; t++ {
			out = append(out, t.String())
		}
		return out, nil
	}
	reg := g.db.Types()
	if reg == nil {
		return nil, fmt.Errorf("engine: ChooseType: Type$ %q needs the type vocabulary", kind)
	}
	switch kind {
	case "Creature":
		return reg.Members(cardtype.CategoryCreature), nil
	case "Basic Land":
		// CardType.getBasicTypes is the BASIC_TYPES HashSet (CardType.java:
		// 940-941), offered in its iteration order.
		return strings.Split(basicTypesHashOrder, " & "), nil
	case "Nonbasic Land":
		return reg.Members(cardtype.CategoryLand), nil
	case "Land":
		return append(reg.Members(cardtype.CategoryBasic), reg.Members(cardtype.CategoryLand)...), nil
	case "Planeswalker":
		return reg.Members(cardtype.CategoryPlaneswalker), nil
	}
	return nil, fmt.Errorf("engine: ChooseType: Type$ %q not resolvable yet", kind)
}

// creatureTypesInTargetedDeck is ChooseTypeEffect.java:80-92, Type$
// CreatureInTargetedDeck: the creature types of every card of each target
// player (Player.getAllCards: battlefield, library, graveyard, hand, exile,
// command, then the stack), first seen first, each once.
func (g *Game) creatureTypesInTargetedDeck(a *Ability) ([]string, error) {
	reg := g.db.Types()
	if reg == nil {
		return nil, fmt.Errorf("engine: ChooseType: Type$ CreatureInTargetedDeck needs the type vocabulary")
	}
	players, err := targetedOrDefinedPlayers(g, a.Controller, a.Source, a.Params, a.refs())
	if err != nil {
		return nil, fmt.Errorf("engine: ChooseType: %w", err)
	}
	var out []string
	for _, pid := range players {
		for _, z := range [...]ZoneType{Battlefield, Library, Graveyard, Hand, Exile, Command, Stack} {
			for _, id := range g.Zone(z, pid).Cards() {
				for _, t := range creatureTypesOf(g.Card(id).Type(), reg) {
					if !containsString(out, t) {
						out = append(out, t)
					}
				}
			}
		}
	}
	return out, nil
}
