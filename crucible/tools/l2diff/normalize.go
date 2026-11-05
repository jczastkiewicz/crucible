package main

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
)

// Side is the engine a GameState text came from.
type Side int

const (
	// GoSide is fixture.Dump written by fixture.Write.
	GoSide Side = iota
	// JavaSide is GameState.toString after GameState.initFromGame.
	JavaSide
)

// javaPhase maps Go's phase spelling (engine phaseNames) to the PhaseType
// constant GameState.toString writes (rule N2).
var javaPhase = map[string]string{
	"Untap": "UNTAP", "Upkeep": "UPKEEP", "Draw": "DRAW", "Main1": "MAIN1",
	"BeginCombat": "COMBAT_BEGIN", "Declare Attackers": "COMBAT_DECLARE_ATTACKERS",
	"Declare Blockers": "COMBAT_DECLARE_BLOCKERS", "First Strike Damage": "COMBAT_FIRST_STRIKE_DAMAGE",
	"Combat Damage": "COMBAT_DAMAGE", "EndCombat": "COMBAT_END", "Main2": "MAIN2",
	"End of Turn": "END_OF_TURN", "Cleanup": "CLEANUP",
}

// JavaPhase is rule N2 for one name: the PhaseType constant for Go's
// spelling, or the name unchanged when it already is one.
func JavaPhase(name string) string {
	if c, ok := javaPhase[name]; ok {
		return c
	}
	return name
}

// zeroKeys are per-player counts both writers may print as 0 (rule N4).
var zeroKeys = []string{"landsplayed", "landsplayedlastturn", "numringtemptedyou", "speed"}

// playerTokens are card tokens whose value names a player (rule N1):
// GameState.getPlayerString writes "P<i>", fixture.Write the player's name.
var playerTokens = []string{"Owner", "EnchantingPlayer", "Protector"}

// refKeys are card tokens whose value is a comma list of card ids (rule N5).
var refKeys = []string{"AttachedTo", "ExiledWith", "RememberedCards", "Imprinting", "ChosenCards"}

// designations are the cards Game.IsDesignationCard names: Java keeps them in
// the command zone, Go writes a key instead (rule N6, ADR-0060).
var designations = []string{"The Monarch", "The Initiative", "The Ring"}

// zoneKeys are the zone suffixes a player key can carry.
var zoneKeys = []string{"hand", "library", "graveyard", "battlefield", "exile", "command", "sideboard"}

// Normalize rewrites one engine's GameState text into the form l2diff
// compares. Each rule is a row of l2-verbs.md's "Normaliser" table:
//
//	N1 Java's player prefixes p0/p1 become human/ai: on keys, on the values of
//	   activeplayer, monarch and initiative, and on a playerTokens token
//	   (P0/P1, case folded).
//	N2 activephase is the PhaseType constant on both sides.
//	N3 Java's printing tokens Set: and Art: are dropped.
//	N4 a key with an empty value, and a zeroKeys count of 0, is dropped.
//	N5 Id: is renumbered in dump order, kept only on a card a refKeys token
//	   names, and every reference is rewritten through the renumbering.
//	N6 designation cards leave the command zone.
//	N7 a card's tokens after its name are sorted.
//	N8 lines are sorted by key.
//	N9 a player who lost keeps only its lost= key: GameState.initFromGame
//	   writes the players still in the game (Game.getPlayers).
func Normalize(text string, side Side) (string, error) {
	type line struct{ key, value string }
	var lines []line
	for _, raw := range strings.Split(text, "\n") {
		raw = strings.TrimSpace(raw)
		if raw == "" || strings.HasPrefix(raw, "#") || strings.HasPrefix(raw, "[") {
			continue
		}
		k, v, ok := strings.Cut(raw, "=")
		if !ok {
			return "", fmt.Errorf("line %q has no '='", raw)
		}
		if side == JavaSide {
			k = javaPlayerKey(k)
			if k == "activeplayer" || k == "monarch" || k == "initiative" {
				v = javaPlayerKey(v)
			}
		}
		if k == "activephase" {
			v = JavaPhase(v)
		}
		lines = append(lines, line{k, v})
	}

	// N5: the ids any reference names, then their new numbers in dump order.
	referenced := map[string]bool{}
	for _, l := range lines {
		if !isZoneKey(l.key) {
			continue
		}
		for _, card := range splitCards(l.value) {
			for _, tok := range strings.Split(card, "|")[1:] {
				k, v, _ := strings.Cut(tok, ":")
				if slices.Contains(refKeys, k) {
					for _, id := range strings.Split(v, ",") {
						referenced[id] = true
					}
				}
			}
		}
	}
	renumber := map[string]string{}
	for _, l := range lines {
		if !isZoneKey(l.key) {
			continue
		}
		for _, card := range splitCards(l.value) {
			for _, tok := range strings.Split(card, "|")[1:] {
				if id, ok := strings.CutPrefix(tok, "Id:"); ok && referenced[id] {
					if _, seen := renumber[id]; !seen {
						renumber[id] = strconv.Itoa(len(renumber) + 1)
					}
				}
			}
		}
	}

	lost := map[string]bool{}
	for _, l := range lines {
		if p, ok := strings.CutSuffix(l.key, "lost"); ok && l.value == "true" {
			lost[p] = true
		}
	}

	var out []string
	for _, l := range lines {
		if p := playerOf(l.key); lost[p] && l.key != p+"lost" && l.key != p+"won" {
			continue
		}
		v := l.value
		if isZoneKey(l.key) {
			v = normalizeZone(l.key, v, side, renumber)
		}
		if v == "" || v == "0" && slices.ContainsFunc(zeroKeys, func(z string) bool { return isPlayerKey(l.key, z) }) {
			continue
		}
		out = append(out, l.key+"="+v)
	}
	slices.Sort(out)
	return strings.Join(out, "\n") + "\n", nil
}

func normalizeZone(key, value string, side Side, renumber map[string]string) string {
	var cards []string
	for _, card := range splitCards(value) {
		toks := strings.Split(card, "|")
		name := toks[0]
		if side == JavaSide && strings.HasSuffix(key, "command") && slices.Contains(designations, name) {
			continue
		}
		var keep []string
		for _, tok := range toks[1:] {
			k, v, _ := strings.Cut(tok, ":")
			switch {
			case k == "Set" || k == "Art":
				continue
			case slices.Contains(playerTokens, k):
				tok = k + ":" + javaPlayerKey(strings.ToLower(v))
			case k == "Id":
				n, ok := renumber[v]
				if !ok {
					continue
				}
				tok = "Id:" + n
			case slices.Contains(refKeys, k):
				ids := strings.Split(v, ",")
				for i, id := range ids {
					if n, ok := renumber[id]; ok {
						ids[i] = n
					}
				}
				tok = k + ":" + strings.Join(ids, ",")
			}
			keep = append(keep, tok)
		}
		slices.Sort(keep)
		cards = append(cards, strings.Join(append([]string{name}, keep...), "|"))
	}
	return strings.Join(cards, ";")
}

func splitCards(v string) []string {
	if v == "" {
		return nil
	}
	return strings.Split(v, ";")
}

// javaPlayerKey is N1: GameState.toString prefixes player i's keys "p<i>",
// and GameState.getPlayerState reads p0 as human and p1 as ai.
func javaPlayerKey(k string) string {
	for prefix, name := range map[string]string{"p0": "human", "p1": "ai"} {
		if rest, ok := strings.CutPrefix(k, prefix); ok {
			return name + rest
		}
	}
	return k
}

// playerOf is the player prefix of a per-player key, "" for a game key.
func playerOf(k string) string {
	for _, p := range []string{"human", "ai", "p2", "p3"} {
		if strings.HasPrefix(k, p) {
			return p
		}
	}
	return ""
}

func isZoneKey(k string) bool {
	return slices.ContainsFunc(zoneKeys, func(z string) bool { return isPlayerKey(k, z) })
}

func isPlayerKey(k, suffix string) bool {
	p := playerOf(k)
	return p != "" && k == p+suffix
}
