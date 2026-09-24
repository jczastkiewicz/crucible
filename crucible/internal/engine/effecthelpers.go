package engine

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/jczastkiewicz/crucible/internal/expr"
	"github.com/jczastkiewicz/crucible/internal/valid"
)

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

// filterValid keeps the cards of ids matching the valid string spec, with
// sourceController as the "You" of that string.
func filterValid(g *Game, ids []CardID, spec string, sourceController PlayerID, source CardID) []CardID {
	parsed := valid.Parse(spec)
	var out []CardID
	for _, id := range ids {
		if Matches(g, g.Card(id), parsed, sourceController, source) {
			out = append(out, id)
		}
	}
	return out
}

// hasParam reports whether a's script names key.
func hasParam(a *Ability, key string) bool {
	_, ok := a.Params.Param(key)
	return ok
}

// withoutCards returns all minus drop, keeping all's order.
func withoutCards(all, drop []CardID) []CardID {
	var out []CardID
	for _, id := range all {
		if !containsCard(drop, id) {
			out = append(out, id)
		}
	}
	return out
}

// reversedCards returns ids in reverse order, a fresh slice.
func reversedCards(ids []CardID) []CardID {
	out := make([]CardID, len(ids))
	for i, id := range ids {
		out[len(ids)-1-i] = id
	}
	return out
}

// swapRememberedPlayer removes every remembered player from m, remembers p
// instead, and returns the removed ones -- the tempRemembered swap
// ChooseGenericEffect and RepeatEachEffect both perform around a
// resolution.
func swapRememberedPlayer(m *Memory, p PlayerID) []EntityID {
	var old []EntityID
	for _, e := range m.Remembered() {
		if _, ok := e.AsPlayer(); ok {
			old = append(old, e)
		}
	}
	for _, e := range old {
		m.Forget(e)
	}
	m.Remember(PlayerEntity(p))
	return old
}

// restoreRememberedPlayers undoes swapRememberedPlayer.
func restoreRememberedPlayers(m *Memory, p PlayerID, old []EntityID) {
	m.Forget(PlayerEntity(p))
	for _, e := range old {
		m.Remember(e)
	}
}

// changeZoneDestination parses Destination$, limited to the five zones a
// card can be moved to by this port (PlanarDeck/Ante/Sideboard/Command/Stack
// are not modeled as destinations).
func changeZoneDestination(name string) (ZoneType, error) {
	z, ok := ZoneByName(name)
	if !ok || (z != Battlefield && z != Graveyard && z != Hand && z != Library && z != Exile) {
		return 0, fmt.Errorf("Destination$ %q not resolvable yet", name)
	}
	return z, nil
}

// rejectParams fails the whole line on the first of keys a names -- the
// PORT-8 "a param this port does not resolve is an error, never a guess"
// gate every effect opens with. api names the effect for the message.
func rejectParams(a *Ability, api string, keys ...string) error {
	for _, key := range keys {
		if _, ok := a.Params.Param(key); ok {
			return fmt.Errorf("engine: %s: %s$ not resolvable yet", api, key)
		}
	}
	return nil
}

// optionalAmount resolves key through resolveNamedAmount, def when absent.
func optionalAmount(g *Game, a *Ability, api, key string, def int) (int, error) {
	raw, ok := a.Params.Param(key)
	if !ok {
		return def, nil
	}
	n, ok := resolveNamedAmount(g, a.Amounts, g.Card(a.Source), raw)
	if !ok {
		return 0, fmt.Errorf("engine: %s: %s$ %q is not resolvable", api, key, raw)
	}
	return n, nil
}

// withAmount is a copy of amounts with name bound to the literal n.
func withAmount(amounts map[string]expr.Amount, name string, n int) map[string]expr.Amount {
	out := make(map[string]expr.Amount, len(amounts)+1)
	for k, v := range amounts {
		out[k] = v
	}
	out[strings.ToLower(name)] = expr.Parse(strconv.Itoa(n))
	return out
}

// battlefieldStaticNames reports whether any permanent has a static ability
// naming key -- how an effect refuses to guess while a static it does not
// model could change it.
func battlefieldStaticNames(g *Game, key string) bool {
	for _, pid := range g.Players() {
		for _, id := range g.Zone(Battlefield, pid).Cards() {
			c := g.Card(id)
			if c.Def == nil {
				continue
			}
			for _, s := range c.Def.Faces[0].Statics {
				if _, ok := s.Param(key); ok {
					return true
				}
			}
		}
	}
	return false
}

// battlefieldStaticMode reports whether any permanent has a static ability
// of Mode$ mode.
func battlefieldStaticMode(g *Game, mode string) bool {
	for _, pid := range g.Players() {
		for _, id := range g.Zone(Battlefield, pid).Cards() {
			c := g.Card(id)
			if c.Def == nil {
				continue
			}
			for _, s := range c.Def.Faces[0].Statics {
				if v, ok := s.Param("Mode"); ok && v == mode {
					return true
				}
			}
		}
	}
	return false
}

// battlefieldReplacementEvent reports whether any permanent has a
// replacement effect for Event$ event.
func battlefieldReplacementEvent(g *Game, event string) bool {
	for _, pid := range g.Players() {
		for _, id := range g.Zone(Battlefield, pid).Cards() {
			c := g.Card(id)
			if c.Def == nil {
				continue
			}
			for _, r := range c.Def.Faces[0].Replacements {
				if v, ok := r.Param("Event"); ok && v == event {
					return true
				}
			}
		}
	}
	return false
}

// containsString reports whether list holds s.
func containsString(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

// rememberAll adds each of objs to m, answering the ones that were new --
// what a later forgetAll takes back (Java's addRemembered/removeRemembered
// pairs around a sub-ability).
func rememberAll(m *Memory, objs []EntityID) []EntityID {
	var added []EntityID
	for _, o := range objs {
		if m.Remember(o) {
			added = append(added, o)
		}
	}
	return added
}

// forgetAll removes each of objs from m.
func forgetAll(m *Memory, objs []EntityID) {
	for _, o := range objs {
		m.Forget(o)
	}
}

// rotateToFront is Collections.rotate bringing first to the front, when
// players holds it.
func rotateToFront(players []PlayerID, first PlayerID) []PlayerID {
	for i, p := range players {
		if p == first {
			return append(append([]PlayerID(nil), players[i:]...), players[:i]...)
		}
	}
	return players
}
