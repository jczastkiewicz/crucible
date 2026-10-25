package engine

//enginelint:allow game control ability effecthelpers card condition defined zone id parts zonemove event putcountereffect amount valid

import (
	"fmt"
	"strings"
)

// makeCardEffect is MakeCardEffect.java: for each Defined$ player (the
// activator by default), cards are made from outside the game -- by Name$
// (ChosenName: the host's named cards), Names$, the printed names of the
// DefinedName$ cards, or one picked from a Spellbook$/Choices$ name list
// (SpellbookAmount$ times; AtRandom$ picks with Aggregates.random). Each
// name is made Amount$ times (default 1), owned by that player, and put
// into Zone$ (default Library; at LibraryPosition$, else the top and then
// the library is shuffled), tapped with Tapped$, entering with WithCounter$
// counters. RememberMade$/ImprintMade$ record them on the host.
type makeCardEffect struct{}

func (makeCardEffect) Resolve(g *Game, a *Ability, controller PlayerController) error {
	if err := rejectUnreadCondition(a, a.Params.Name); err != nil {
		return err
	}
	if err := rejectParams(a, "MakeCard", "Booster"); err != nil {
		return err
	}
	source := g.Card(a.Source)
	if !subAbilityConditionMet(g, source, a.Amounts, a.Params) {
		return nil
	}
	def, ok := a.Params.Param("Defined")
	if !ok {
		def = "You"
	}
	players, err := definedPlayers(g, a.Controller, a.Source, def, a.refs())
	if err != nil {
		return fmt.Errorf("engine: MakeCard: %w", err)
	}
	zone := Library
	if raw, ok := a.Params.Param("Zone"); ok {
		z, ok := ZoneByName(raw)
		if !ok {
			return fmt.Errorf("engine: MakeCard: Zone$ %q not resolvable", raw)
		}
		zone = z
	}
	// AttachedTo$ (MakeCardEffect.java:122-142): the cards enter the battlefield,
	// each attached to every host named, a Defined$ card list or else the
	// battlefield cards the spec matches. No host ends the effect.
	var hosts []CardID
	if spec, ok := a.Params.Param("AttachedTo"); ok {
		zone = Battlefield
		if hosts, err = definedCards(source, spec, a.refs()); err != nil || len(hosts) == 0 {
			hosts = g.battlefieldMatching(spec, a.Controller, a.Source)
		}
		if len(hosts) == 0 {
			return nil
		}
	}
	for _, p := range players {
		if hasParam(a, "Optional") && hasParam(a, "OptionPrompt") && !controller.ConfirmEffect(g, p, a.Source) {
			return nil
		}
		names, err := makeCardNames(g, a, controller, source, p)
		if err != nil {
			return err
		}
		if hasParam(a, "Filter") && len(names) == 0 {
			// MakeCardEffect.java:102: a filter that keeps no face ends this
			// player's turn at it, with no move, trigger or shuffle.
			continue
		}
		// Amount is read inside the player loop (MakeCardEffect.java:129-130).
		amount, err := optionalAmount(g, a, "MakeCard", "Amount", 1)
		if err != nil {
			return err
		}
		var made []CardID
		var attachHosts []CardID
		for _, name := range names {
			if name == "" {
				continue
			}
			cardDef, ok := g.db.Card(name)
			if !ok {
				return fmt.Errorf("engine: MakeCard: no card named %q", name)
			}
			for i := 0; i < amount; i++ {
				id := g.NewCard(cardDef, p, None)
				if hasParam(a, "TokenCard") {
					// MakeCardEffect.java:161-163: setTokenCard, not a token
					// (isToken reads gamePieceType); only the "token" valid
					// property sees it (CardProperty.java:1346-1347).
					g.Card(id).IsTokenCard = true
				}
				if hasParam(a, "Tapped") {
					g.Card(id).Tapped = true
				}
				made = append(made, id)
			}
		}
		if len(hosts) > 0 {
			// One card per host: the made card for the first, a fresh copy for
			// each other (MakeCardEffect.java:186-194).
			var perHost []CardID
			var hostOf []CardID
			for _, id := range made {
				for i, h := range hosts {
					copyID := id
					if i > 0 {
						copyID = g.NewCard(g.Card(id).Def, p, None)
					}
					perHost = append(perHost, copyID)
					hostOf = append(hostOf, h)
				}
			}
			made = perHost
			attachHosts = hostOf
		}
		counterKind, withCounter := a.Params.Param("WithCounter")
		counterType := CounterType(strings.ToUpper(counterKind))
		for madeIdx, id := range made {
			// The counters go through AddCounter replacements and a Mode$
			// CantPutCounter static either way (GameEntityCounterTable
			// .replaceCounterEffect): entering with them on the battlefield, put
			// on the card where it landed anywhere else.
			// WithCounterNum is read per made card (MakeCardEffect.java:180, :205).
			counterNum := 0
			if withCounter {
				if counterNum, err = optionalAmount(g, a, "MakeCard", "WithCounterNum", 1); err != nil {
					return err
				}
			}
			var enter []enterCounters
			if withCounter && zone == Battlefield {
				enter = append(enter, enterCounters{kind: counterType, n: counterNum, placer: p})
			}
			// calculateAmount runs per made card (MakeCardEffect.java:200), so a
			// Count$Random position re-rolls for each.
			libPos, err := libraryPosition(g, a, source)
			if err != nil {
				return fmt.Errorf("engine: MakeCard: %w", err)
			}
			g.moveByEffect(controller, id, zone, libPos, NoPlayer, g.Card(id).Tapped, enter...)
			if withCounter && zone != Battlefield {
				// made.addCounter(..., player, counterTable), placed by
				// replaceCounterEffect (MakeCardEffect.java:203-206, :211):
				// CounterAdded fires for them too.
				if n := g.countersReplaced(controller, p, CardEntity(id), counterType, counterNum); n > 0 {
					g.addCardCounters(controller, p, id, id, counterType, n)
				}
			}
			if attachHosts != nil && g.Card(id).Zone == Battlefield && g.Card(attachHosts[madeIdx]).Zone == Battlefield {
				g.attachTo(controller, id, attachHosts[madeIdx])
			}
			if hasParam(a, "FaceDown") {
				// finishMaking (MakeCardEffect.java:246-247) turns the card face
				// down after the move and its counters, so a trigger on the
				// arrival saw it face up (PORT-7).
				if g.Card(id).Zone == Exile {
					g.turnFaceDownInExile(id)
				} else {
					g.turnFaceDown(id)
				}
			}
			if hasParam(a, "RememberMade") {
				source.Memory.Remember(CardEntity(id))
			}
			if hasParam(a, "ImprintMade") {
				source.Memory.Imprint(id)
			}
		}
		g.checkChangesZoneAllTriggers(controller, made, None, zone)
		if hasParam(a, "Conjure") {
			// MakeCardEffect.java:219-224: once per player, before the shuffle.
			g.checkConjureAllTriggers(controller, p, made)
		}
		if zone == Library && !hasParam(a, "LibraryPosition") {
			g.Shuffle(Library, p)
		}
	}
	return nil
}

// makeCardNames is MakeCardEffect's name-gathering half for player p.
func makeCardNames(g *Game, a *Ability, controller PlayerController, source *Card, p PlayerID) ([]string, error) {
	if n, ok := a.Params.Param("Name"); ok {
		if n == "ChosenName" {
			return append([]string(nil), source.Memory.NamedCards()...), nil
		}
		return []string{n}, nil
	}
	if raw, ok := a.Params.Param("Names"); ok {
		var out []string
		for _, s := range strings.Split(raw, ",") {
			out = append(out, strings.ReplaceAll(s, ";", ","))
		}
		return out, nil
	}
	if def, ok := a.Params.Param("DefinedName"); ok {
		var cards []CardID
		if def == "ChosenMap" {
			// MakeCardEffect.java:68-69: the cards p chose into the host's map.
			var has bool
			if cards, has = source.Memory.ChosenMapFor(p); !has {
				return nil, fmt.Errorf("engine: MakeCard: DefinedName$ ChosenMap: player %d chose nothing into the map", p)
			}
		} else {
			var err error
			if cards, err = definedCards(source, def, a.refs()); err != nil {
				return nil, fmt.Errorf("engine: MakeCard: %w", err)
			}
		}
		var out []string
		for _, id := range cards {
			if d := g.Card(id).Def; d != nil {
				out = append(out, d.Name)
			}
		}
		return out, nil
	}
	key := "Spellbook"
	raw, ok := a.Params.Param(key)
	if !ok {
		key = "Choices"
		if raw, ok = a.Params.Param(key); !ok {
			return nil, nil
		}
	}
	var faces []string
	for _, s := range strings.Split(raw, ",") {
		faces = append(faces, strings.ReplaceAll(strings.TrimSpace(s), ";", ","))
	}
	if filter, ok := a.Params.Param("Filter"); ok {
		var err error
		if faces, err = filterMakeCardFaces(g, a, source, faces, filter); err != nil {
			return nil, err
		}
	}
	times, err := optionalAmount(g, a, "MakeCard", "SpellbookAmount", 1)
	if err != nil {
		return nil, err
	}
	var out []string
	for ; times > 0 && len(faces) > 0; times-- {
		var i int
		if hasParam(a, "AtRandom") {
			i = g.randomIndex(len(faces))
		} else {
			i = controller.ChooseOption(g, p, a.Source, faces)
			if i < 0 || i >= len(faces) {
				return nil, fmt.Errorf("engine: MakeCard: %s$ choice %d out of range", key, i)
			}
		}
		out = append(out, faces[i])
		faces = append(faces[:i:i], faces[i+1:]...)
	}
	return out, nil
}

// filterMakeCardFaces is MakeCardEffect.java:94-103: the faces whose card
// passes Filter$, evaluated by Card.isValid against a fresh card of that name
// with the host as source. Only "Card.cmc<OP><amount>" is read (both corpus
// lines, Brazen Boarding and Tajic); the amount is calculated against the host
// (CardProperty.java:1450-1477). Filter$ is never read on the Name$ paths in
// Java, so they ignore it here too (PORT-7).
func filterMakeCardFaces(g *Game, a *Ability, source *Card, faces []string, filter string) ([]string, error) {
	rest, ok := strings.CutPrefix(filter, "Card.cmc")
	if !ok || len(rest) < 3 {
		return nil, fmt.Errorf("engine: MakeCard: Filter$ %q not resolvable yet", filter)
	}
	op, rhs := rest[:2], rest[2:]
	want, ok := resolveNamedAmount(g, a.Amounts, source, rhs)
	if !ok {
		return nil, fmt.Errorf("engine: MakeCard: Filter$ %q: %q is not resolvable", filter, rhs)
	}
	var kept []string
	for _, name := range faces {
		def, ok := g.db.Card(name)
		if !ok {
			return nil, fmt.Errorf("engine: MakeCard: no card named %q", name)
		}
		if compareOp(def.Faces[0].ManaCost.CMC(), op, want) {
			kept = append(kept, name)
		}
	}
	return kept, nil
}
