// Building a *engine.Game from a parsed fixture.

package fixture

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/jczastkiewicz/crucible/internal/carddb/compile"
	"github.com/jczastkiewicz/crucible/internal/engine"
	"github.com/jczastkiewicz/crucible/pkg/javarand"
)

// Loaded is what Load produces: the engine.Game plus the fixture-level state
// that has no home in the engine yet, because the turn/phase loop that would
// own it -- Forge's PhaseHandler -- is M5, not built (Plan Section 3.2, P4).
type Loaded struct {
	Game *engine.Game

	Turn               int
	ActivePlayer       engine.PlayerID
	ActivePhase        engine.PhaseType
	Phased             bool
	ActivePhaseAdvance engine.PhaseType
	PhaseAdvanced      bool

	// Unapplied records every value Load recognised the shape of but had
	// nothing to apply it to: a card annotation for a mechanic that is not
	// modeled yet (Renowned, ChosenColor, ...), or a player-level field with
	// no corresponding engine.Player field (ManaPool, LandsPlayed, ...).
	// Silently dropping these would make a fixture that names, say, a
	// Monstrous creature pass while testing something other than what it
	// says.
	Unapplied []string
}

// Load builds a game from a parsed fixture.
//
// Ported from GameState.applyToGame and processCardsForZone, with one
// structural difference: Java applies fixture state onto a Game a Match
// already built, with its players already seated. Crucible has no Match yet,
// so Load builds the Game itself, seating one player per Named slot in slot
// order -- human, ai, p2..p9 -- which is the same seating order every fixture
// already writes in (PORT-1).
func Load(st *State, db *compile.DB, rng *javarand.Rand) (*Loaded, error) {
	var names []string
	var slots []int
	for i, p := range st.Players {
		if p.Named {
			names = append(names, slotName(i))
			slots = append(slots, i)
		}
	}
	g := engine.NewGame(db, rng, names)

	var slotToID [MaxPlayers]engine.PlayerID
	for i, slot := range slots {
		slotToID[slot] = g.Players()[i]
	}

	l := &Loaded{
		Game:               g,
		Turn:               st.Turn,
		ActivePhase:        st.ActivePhase,
		Phased:             st.Phased,
		ActivePhaseAdvance: st.ActivePhaseAdvance,
		PhaseAdvanced:      st.PhaseAdvanced,
	}
	if st.ActivePlayer != "" {
		slot, ok := playerSlot(st.ActivePlayer)
		if !ok || slotToID[slot] == engine.NoPlayer {
			return nil, fmt.Errorf("activeplayer %q: no such player", st.ActivePlayer)
		}
		l.ActivePlayer = slotToID[slot]
	}

	ld := &loader{game: g, slotToID: slotToID, idToCard: map[int]engine.CardID{}}
	for _, slot := range slots {
		ps := &st.Players[slot]
		pid := slotToID[slot]
		g.Player(pid).Life = ps.Life
		if ps.Counters != "" {
			l.Unapplied = append(l.Unapplied, fmt.Sprintf("%s: counters %q -- engine.Player has no counters yet", slotName(slot), ps.Counters))
		}
		if ps.ManaPool != "" || ps.PersistentMana != "" {
			l.Unapplied = append(l.Unapplied, fmt.Sprintf("%s: mana pool -- engine.Player has no mana pool yet", slotName(slot)))
		}
		if ps.LandsPlayed != 0 || ps.LandsPlayedLastTurn != 0 {
			l.Unapplied = append(l.Unapplied, fmt.Sprintf("%s: lands played -- engine.Player has no lands-played count yet", slotName(slot)))
		}

		for _, z := range []struct {
			text string
			kind engine.ZoneType
		}{
			{ps.Battlefield, engine.Battlefield},
			{ps.Hand, engine.Hand},
			{ps.Graveyard, engine.Graveyard},
			{ps.Library, engine.Library},
			{ps.Exile, engine.Exile},
			{ps.Command, engine.Command},
			{ps.Sideboard, engine.Sideboard},
		} {
			if err := ld.zone(z.text, z.kind, pid); err != nil {
				return nil, err
			}
		}
	}

	if err := ld.resolveRefs(); err != nil {
		return nil, err
	}
	if st.RemoveSummoningSickness {
		for _, p := range slots {
			for _, id := range g.Zone(engine.Battlefield, slotToID[p]).Cards() {
				g.Card(id).SummonSick = false
			}
		}
	}
	l.Unapplied = append(l.Unapplied, ld.unapplied...)
	return l, nil
}

// slotName is the inverse of playerSlot: the name Load seats a player under.
func slotName(slot int) string {
	switch slot {
	case 0:
		return "human"
	case 1:
		return "ai"
	default:
		return fmt.Sprintf("p%d", slot)
	}
}

// loader carries the state that spans every zone and every player while
// Load builds the game: the fixture's declared card IDs, and the
// cross-references that can only resolve once every card exists.
type loader struct {
	game     *engine.Game
	slotToID [MaxPlayers]engine.PlayerID
	idToCard map[int]engine.CardID

	// Resolved after every card in the fixture has been created, and in the
	// order the fixture declared them: two auras naming the same host must
	// attach in that order, because it is what breaks a tie between their
	// continuous effects when both share a timestamp (GO-12).
	attaches  []attachRef
	remembers []refList
	imprints  []refList
	unapplied []string
}

type attachRef struct {
	card   engine.CardID
	hostID int
}

type refList struct {
	card engine.CardID
	ids  []int
}

// zone creates every card a zone's raw text names, in the order written.
func (ld *loader) zone(text string, kind engine.ZoneType, owner engine.PlayerID) error {
	if text == "" {
		return nil
	}
	for _, entry := range strings.Split(text, ";") {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		if err := ld.card(entry, kind, owner); err != nil {
			return err
		}
	}
	return nil
}

// card creates one card from a zone entry -- the card's name, then its
// `|`-separated annotations -- and files any cross-references it makes for
// resolveRefs to apply once every card exists.
func (ld *loader) card(entry string, kind engine.ZoneType, owner engine.PlayerID) error {
	fields := strings.Split(entry, "|")
	name := fields[0]
	if strings.HasPrefix(name, "t:") || strings.HasPrefix(name, "T:") {
		ld.unapplied = append(ld.unapplied, fmt.Sprintf("%s: token cards are not loaded yet", name))
		return nil
	}
	def, ok := ld.game.DB().Card(name)
	if !ok {
		return fmt.Errorf("card %q: not in the database", name)
	}
	id := ld.game.NewCard(def, owner, kind)
	c := ld.game.Card(id)

	var remembered, imprinted []int
	for _, info := range fields[1:] {
		switch {
		// Set: and Art: pick a printing. Crucible plays cards, not printings
		// (porting/port-log/deck-serializer.md), so both parse and are
		// dropped rather than reported -- that is an established decision,
		// not a gap.
		case strings.HasPrefix(info, "Set:"), strings.HasPrefix(info, "Art:"):
		// Tapped and SummonSick match on prefix with no colon required,
		// which is Java's own info.startsWith("Tapped") -- reproduced
		// because a fixture written either way has to load the same in both
		// engines (PORT-7).
		case strings.HasPrefix(info, "Tapped"):
			c.Tapped = true
		case strings.HasPrefix(info, "SummonSick"):
			c.SummonSick = true
		case strings.HasPrefix(info, "Counters:"):
			if err := applyCounters(c, strings.TrimPrefix(info, "Counters:")); err != nil {
				return fmt.Errorf("%s: %w", name, err)
			}
		case strings.HasPrefix(info, "Damage:"):
			n, err := strconv.Atoi(strings.TrimPrefix(info, "Damage:"))
			if err != nil {
				return fmt.Errorf("%s: damage %q: %w", name, info, err)
			}
			c.Damage.Mark(n, false)
		case strings.HasPrefix(info, "Id:"):
			n, err := strconv.Atoi(strings.TrimPrefix(info, "Id:"))
			if err != nil {
				return fmt.Errorf("%s: id %q: %w", name, info, err)
			}
			ld.idToCard[n] = id
		case strings.HasPrefix(info, "AttachedTo:"), strings.HasPrefix(info, "Attaching:"):
			n, err := strconv.Atoi(info[strings.Index(info, ":")+1:])
			if err != nil {
				return fmt.Errorf("%s: attach %q: %w", name, info, err)
			}
			ld.attaches = append(ld.attaches, attachRef{id, n})
		case strings.HasPrefix(info, "Owner:"):
			slot, ok := playerSlot(strings.ToLower(strings.TrimSpace(strings.TrimPrefix(info, "Owner:"))))
			if !ok {
				return fmt.Errorf("%s: owner %q: no such player", name, info)
			}
			c.Owner = ld.slotToID[slot]
		case strings.HasPrefix(info, "RememberedCards:"):
			ids, err := parseIDList(strings.TrimPrefix(info, "RememberedCards:"))
			if err != nil {
				return fmt.Errorf("%s: %w", name, err)
			}
			remembered = ids
		case strings.HasPrefix(info, "Imprinting:"):
			ids, err := parseIDList(strings.TrimPrefix(info, "Imprinting:"))
			if err != nil {
				return fmt.Errorf("%s: %w", name, err)
			}
			imprinted = ids
		default:
			ld.unapplied = append(ld.unapplied, fmt.Sprintf("%s: %s", name, info))
		}
	}
	if remembered != nil {
		ld.remembers = append(ld.remembers, refList{id, remembered})
	}
	if imprinted != nil {
		ld.imprints = append(ld.imprints, refList{id, imprinted})
	}
	return nil
}

// resolveRefs applies every cross-reference collected while cards were being
// created. It runs once every card in the fixture exists, because a card can
// reference one declared later in the file.
func (ld *loader) resolveRefs() error {
	for _, a := range ld.attaches {
		host, ok := ld.idToCard[a.hostID]
		if !ok {
			return fmt.Errorf("attachedto %d: no card has that id", a.hostID)
		}
		ld.game.Attach(a.card, host)
	}
	for _, r := range ld.remembers {
		for _, refID := range r.ids {
			target, ok := ld.idToCard[refID]
			if !ok {
				return fmt.Errorf("rememberedcards %d: no card has that id", refID)
			}
			ld.game.Card(r.card).Memory.Remember(engine.CardEntity(target))
		}
	}
	for _, r := range ld.imprints {
		for _, refID := range r.ids {
			target, ok := ld.idToCard[refID]
			if !ok {
				return fmt.Errorf("imprinting %d: no card has that id", refID)
			}
			ld.game.Card(r.card).Memory.Imprint(target)
		}
	}
	return nil
}

// applyCounters parses Counters:'s "TYPE=n,TYPE=n" value, the same format
// Player-level counters use.
func applyCounters(c *engine.Card, value string) error {
	for _, pair := range strings.Split(value, ",") {
		typ, n, ok := strings.Cut(pair, "=")
		if !ok {
			return fmt.Errorf("counter %q: missing =", pair)
		}
		count, err := strconv.Atoi(strings.TrimSpace(n))
		if err != nil {
			return fmt.Errorf("counter %q: %w", pair, err)
		}
		c.Counters.Add(engine.CounterType(strings.TrimSpace(typ)), count)
	}
	return nil
}

// parseIDList parses a comma-separated list of fixture card IDs.
func parseIDList(value string) ([]int, error) {
	parts := strings.Split(value, ",")
	ids := make([]int, len(parts))
	for i, p := range parts {
		n, err := strconv.Atoi(strings.TrimSpace(p))
		if err != nil {
			return nil, fmt.Errorf("id list %q: %w", value, err)
		}
		ids[i] = n
	}
	return ids, nil
}
