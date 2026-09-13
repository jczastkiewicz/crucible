// Writing a *engine.Game back to a fixture.

package fixture

import (
	"strconv"
	"strings"

	"github.com/jczastkiewicz/crucible/internal/engine"
)

// Dump reconstructs a fixture from a loaded game.
//
// It assumes the game's players are named the way Load seats them -- human,
// ai, p2..p9 -- because that is the only way to recover which slot a
// engine.PlayerID belongs to: Load builds the name from the slot, and Dump
// reads it back rather than carrying a second, parallel mapping alongside the
// Game (PORT-1).
//
// A card's Id: is its own CardID, not a number the fixture chose. Java's
// GameState hands out arbitrary ids and only writes one when something else
// references that card; Go always writes one and always uses the handle it
// already has, because there is nothing to gain from inventing a second
// numbering scheme that Load would have to remember, and Parse(Dump(x)) does
// not need to reproduce x's own numbers to prove the round trip works.
func Dump(l *Loaded) *State {
	g := l.Game
	st := &State{
		Turn:               g.Turn(),
		ActivePhaseAdvance: l.ActivePhaseAdvance,
		PhaseAdvanced:      l.PhaseAdvanced,
		// RemoveSummoningSickness is a one-time load directive, not state a
		// loaded game remembers being given. Its effect already survived: by
		// the time Load returns, every card it applied to already has
		// SummonSick false, and dumpCard writes that per card. Re-emitting
		// the directive here would be asserting something Dump cannot
		// actually know.
		RemoveSummoningSickness: false,
	}
	// ActivePhase has no "unset" of its own on Game -- a real game always has
	// some active phase once its turn state is initialised. Emitting it only
	// alongside ActivePlayer keeps a truly empty fixture (no active player at
	// all) from gaining a phantom activephase=Untap line it never wrote.
	if g.ActivePlayer() != engine.NoPlayer {
		st.ActivePlayer = g.Player(g.ActivePlayer()).Name
		st.ActivePhase = g.ActivePhase()
		st.Phased = true
	}

	for _, pid := range g.Players() {
		slot, ok := playerSlot(g.Player(pid).Name)
		if !ok {
			continue
		}
		ps := &st.Players[slot]
		ps.Named = true
		ps.Life = g.Player(pid).Life

		ps.Battlefield = dumpZone(g, engine.Battlefield, pid)
		ps.Hand = dumpZone(g, engine.Hand, pid)
		ps.Graveyard = dumpZone(g, engine.Graveyard, pid)
		ps.Library = dumpZone(g, engine.Library, pid)
		ps.Exile = dumpZone(g, engine.Exile, pid)
		ps.Command = dumpZone(g, engine.Command, pid)
		ps.Sideboard = dumpZone(g, engine.Sideboard, pid)
	}
	return st
}

// dumpZone writes one zone's cards as a `;`-joined entry list, in the zone's
// own order (GO-12): that order is what a fixture reproduces when it names a
// library, so Dump must not reorder it.
func dumpZone(g *engine.Game, kind engine.ZoneType, owner engine.PlayerID) string {
	cards := g.Zone(kind, owner).Cards()
	if len(cards) == 0 {
		return ""
	}
	entries := make([]string, len(cards))
	for i, id := range cards {
		entries[i] = dumpCard(g, id)
	}
	return strings.Join(entries, ";")
}

// dumpCard writes one card's `|`-separated entry. The zone gates Java's
// GameState.toString applies stand as written: Tapped, SummonSick, Owner,
// AttachedTo, Damage, RememberedCards and Imprinting are Battlefield-only;
// Counters is Battlefield or Exile; Id has no gate.
func dumpCard(g *engine.Game, id engine.CardID) string {
	c := g.Card(id)
	var b strings.Builder
	b.WriteString(c.Def.Name)
	b.WriteString("|Id:")
	b.WriteString(strconv.FormatUint(uint64(id), 10))

	if c.Zone == engine.Battlefield {
		if c.Owner != c.Controller {
			b.WriteString("|Owner:")
			b.WriteString(g.Player(c.Owner).Name)
		}
		if c.Tapped {
			b.WriteString("|Tapped")
		}
		if c.SummonSick {
			b.WriteString("|SummonSick")
		}
		if host, ok := c.AttachedTo(); ok {
			b.WriteString("|AttachedTo:")
			b.WriteString(strconv.FormatUint(uint64(host), 10))
		}
		if c.Damage.Marked > 0 {
			b.WriteString("|Damage:")
			b.WriteString(strconv.Itoa(c.Damage.Marked))
		}
		if ids := cardRefs(c.Memory.Remembered()); len(ids) > 0 {
			b.WriteString("|RememberedCards:")
			b.WriteString(strings.Join(ids, ","))
		}
		if imp := c.Memory.Imprinted(); len(imp) > 0 {
			b.WriteString("|Imprinting:")
			b.WriteString(joinCardIDs(imp))
		}
	}
	if c.Zone == engine.Battlefield || c.Zone == engine.Exile {
		if s := dumpCounters(c); s != "" {
			b.WriteString("|Counters:")
			b.WriteString(s)
		}
	}
	return b.String()
}

// cardRefs keeps the card entities out of a Remembered list, which can also
// hold players (PlayerEntity), and renders the rest as RememberedCards:'s
// comma id list.
func cardRefs(entities []engine.EntityID) []string {
	var ids []string
	for _, e := range entities {
		if id, ok := e.AsCard(); ok {
			ids = append(ids, strconv.FormatUint(uint64(id), 10))
		}
	}
	return ids
}

func joinCardIDs(ids []engine.CardID) string {
	out := make([]string, len(ids))
	for i, id := range ids {
		out[i] = strconv.FormatUint(uint64(id), 10)
	}
	return strings.Join(out, ",")
}

// dumpCounters renders a card's counters as Counters:'s "TYPE=n,TYPE=n"
// value, in the order they were first put on the card.
func dumpCounters(c *engine.Card) string {
	kinds := c.Counters.Kinds()
	if len(kinds) == 0 {
		return ""
	}
	parts := make([]string, len(kinds))
	for i, k := range kinds {
		parts[i] = string(k) + "=" + strconv.Itoa(c.Counters.Count(k))
	}
	return strings.Join(parts, ",")
}
