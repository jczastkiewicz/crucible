// Package fixture reads Forge's game-state text format.
//
// A fixture is a whole board written as `key=value` lines. Crucible's rules
// tests are directories of them (TEST-5), and the format is Forge's own so the
// same fixture can be run against the Java oracle -- which is the point: a
// scenario that only Crucible can load proves nothing about parity.
//
// Ported from forge-game/src/main/java/forge/game/GameState.java.
package fixture

import (
	"bufio"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"

	"github.com/jczastkiewicz/crucible/internal/engine"
)

// MaxPlayers is how many player slots the format can name. `p0` through `p9`,
// because Java reads a single character after the `p`.
const MaxPlayers = 10

// State is a parsed fixture: the global directives, and one slot per player.
//
// Values are kept as written. Turning a card list into cards needs the
// database and the arena, so it belongs to the loader that builds a game, not
// to the parser that reads the file.
type State struct {
	// Turn is the turn number, zero when the fixture does not say.
	Turn int
	// ActivePlayer is the player whose turn it is, as written: `human`, `ai`,
	// or a `p<n>` name. Empty when the fixture does not say.
	ActivePlayer string
	// ActivePhase is the phase the fixture starts in, and Phased reports
	// whether it named one -- Untap is a real phase, so the zero value cannot
	// stand for "unset".
	ActivePhase engine.PhaseType
	Phased      bool
	// ActivePhaseAdvance is a second phase to advance to after setup, used by
	// scenarios that need the engine to run a step before the fixture's
	// actions start. PhaseAdvanced reports whether the fixture named one.
	ActivePhaseAdvance engine.PhaseType
	PhaseAdvanced      bool
	// RemoveSummoningSickness applies to every creature on the battlefield.
	RemoveSummoningSickness bool
	// Players is indexed by slot: 0 is `human`, 1 is `ai`, and `p<n>` is n.
	Players [MaxPlayers]PlayerState
	// AbilityStrings holds `ability<key>=` lines verbatim, keyed by what
	// follows `ability`. Puzzle-mode fixtures use these to resolve targets for
	// a precast spell; unlike every other category this one is not
	// player-scoped in Java, so it is not part of PlayerState.
	AbilityStrings map[string]string
	// Unknown collects keys the parser recognised the shape of but not the
	// category. Java prints these to stderr and continues; a fixture that
	// silently ignores a line is a test asserting something other than what it
	// says, so they are returned instead.
	Unknown []string
}

// PlayerState is one player's half of a fixture. Zone contents are the raw
// value: a `|`-separated card list the loader parses.
type PlayerState struct {
	Named bool // whether any line addressed this slot

	Life                int
	Counters            string
	LandsPlayed         int
	LandsPlayedLastTurn int
	NumRingTemptedYou   int
	Speed               int

	Battlefield string
	Hand        string
	Graveyard   string
	Library     string
	Exile       string
	Command     string
	Sideboard   string

	ManaPool       string
	PersistentMana string
	PrecastSpells  string
	PutOnStack     string
}

// Parse reads a fixture.
//
// Line handling is Java's, quirks included (PORT-7): a `#` comment is dropped,
// a line with no `=` is dropped, and the split takes the first `=` only, so a
// value may contain more.
//
// One deliberate difference. Java calls line.charAt(0) before testing for a
// comment, so a blank line raises StringIndexOutOfBoundsException and kills
// the load. Reproducing a crash would make blank lines unwritable in Crucible's
// own fixtures for no benefit, so a blank line is skipped and noted here
// rather than silently diverging.
func Parse(r io.Reader) (*State, error) {
	st := &State{}
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)

	for line := 1; sc.Scan(); line++ {
		text := sc.Text()
		if strings.TrimSpace(text) == "" || strings.HasPrefix(text, "#") {
			continue
		}
		key, value, ok := strings.Cut(text, "=")
		if !ok {
			continue
		}
		if err := st.apply(strings.ToLower(key), value); err != nil {
			return nil, fmt.Errorf("line %d: %w", line, err)
		}
	}
	return st, sc.Err()
}

// Write serialises a fixture as `key=value` lines, in the order Load and
// Parse both read them back in the same shape: global directives, then one
// player's life and zones per Named slot, then ability strings.
//
// It is not required to match Java's own key order or line count byte for
// byte -- that would mean carrying Java's exact field-write order as a second
// spec to track. What it must do, and is tested to do, is round-trip:
// Parse(Write(x)) reproduces every field Load and Dump can see in x.
func Write(w io.Writer, s *State) error {
	var b strings.Builder
	if s.Turn != 0 {
		fmt.Fprintf(&b, "turn=%d\n", s.Turn)
	}
	if s.ActivePlayer != "" {
		fmt.Fprintf(&b, "activeplayer=%s\n", s.ActivePlayer)
	}
	if s.Phased {
		fmt.Fprintf(&b, "activephase=%s\n", s.ActivePhase)
	}
	if s.PhaseAdvanced {
		fmt.Fprintf(&b, "activephaseadvance=%s\n", s.ActivePhaseAdvance)
	}
	if s.RemoveSummoningSickness {
		b.WriteString("removesummoningsickness=true\n")
	}

	for i := range s.Players {
		p := &s.Players[i]
		if !p.Named {
			continue
		}
		name := slotName(i)
		fmt.Fprintf(&b, "%slife=%d\n", name, p.Life)
		writeZone(&b, name, "battlefield", p.Battlefield)
		writeZone(&b, name, "hand", p.Hand)
		writeZone(&b, name, "graveyard", p.Graveyard)
		writeZone(&b, name, "library", p.Library)
		writeZone(&b, name, "exile", p.Exile)
		writeZone(&b, name, "command", p.Command)
		writeZone(&b, name, "sideboard", p.Sideboard)
	}

	for _, key := range sortedKeys(s.AbilityStrings) {
		fmt.Fprintf(&b, "ability%s=%s\n", key, s.AbilityStrings[key])
	}

	_, err := w.Write([]byte(b.String()))
	return err
}

// writeZone emits a zone line only when the fixture has something to say
// about it -- an absent zone and an empty one both parse back the same way
// (TestEmptyValueIsAValue), so Write need not distinguish them either.
func writeZone(b *strings.Builder, player, zone, value string) {
	if value == "" {
		return
	}
	fmt.Fprintf(b, "%s%s=%s\n", player, zone, value)
}

// sortedKeys gives AbilityStrings a deterministic write order. Iteration
// order over the map itself carries no meaning -- Java's own abilityString
// map is a plain HashMap -- so sorting is Write picking an order, not
// reproducing one.
func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// apply routes one line. The order is Java's: the global directives are tested
// before the player-scoped ones, and the player-scoped categories are matched
// on the suffix of the whole key rather than on the part after the prefix.
func (s *State) apply(key, value string) error {
	switch {
	case strings.HasPrefix(key, "active"):
		return s.applyActive(key, value)
	case key == "turn":
		n, err := strconv.Atoi(strings.TrimSpace(value))
		if err != nil {
			return fmt.Errorf("turn %q: %w", value, err)
		}
		s.Turn = n
		return nil
	case key == "removesummoningsickness":
		s.RemoveSummoningSickness = strings.EqualFold(strings.TrimSpace(value), "true")
		return nil
	case strings.HasPrefix(key, "ability"):
		if s.AbilityStrings == nil {
			s.AbilityStrings = make(map[string]string)
		}
		s.AbilityStrings[strings.TrimPrefix(key, "ability")] = value
		return nil
	}

	slot, ok := playerSlot(key)
	if !ok {
		s.Unknown = append(s.Unknown, key)
		return nil
	}
	return s.applyPlayer(slot, key, value)
}

func (s *State) applyActive(key, value string) error {
	v := strings.ToLower(strings.TrimSpace(value))
	switch {
	case strings.HasSuffix(key, "player"):
		s.ActivePlayer = v
	case strings.HasSuffix(key, "phaseadvance"):
		p, ok := phaseByName(value)
		if !ok {
			return fmt.Errorf("unknown phase %q", value)
		}
		s.ActivePhaseAdvance, s.PhaseAdvanced = p, true
	case strings.HasSuffix(key, "phase"):
		p, ok := phaseByName(value)
		if !ok {
			return fmt.Errorf("unknown phase %q", value)
		}
		s.ActivePhase, s.Phased = p, true
	default:
		s.Unknown = append(s.Unknown, key)
	}
	return nil
}

// javaPhaseNames are Java's PhaseType enum constant names, in the same
// declaration order as engine.PhaseType (and so as engine's own unexported
// phaseNames table). GameState.toString writes these -- bare Enum#toString,
// not PhaseType.nameForScripts -- so a fixture dumped by the Java oracle uses
// this vocabulary, not the script one engine.PhaseByName matches.
var javaPhaseNames = [...]string{
	"UNTAP", "UPKEEP", "DRAW", "MAIN1", "COMBAT_BEGIN",
	"COMBAT_DECLARE_ATTACKERS", "COMBAT_DECLARE_BLOCKERS",
	"COMBAT_FIRST_STRIKE_DAMAGE", "COMBAT_DAMAGE", "COMBAT_END",
	"MAIN2", "END_OF_TURN", "CLEANUP",
}

// phaseByName accepts either vocabulary PhaseType.smartValueOf does: the
// script name a card's `Phase$` value writes, or the enum constant name a
// Java-dumped GameState blob actually contains. A hand-written fixture uses
// the former; the differential harness's own goldens use the latter.
func phaseByName(value string) (engine.PhaseType, bool) {
	if p, ok := engine.PhaseByName(strings.TrimSpace(value)); ok {
		return p, true
	}
	v := strings.ToUpper(strings.TrimSpace(value))
	for i, n := range javaPhaseNames {
		if n == v {
			return engine.PhaseType(i), true
		}
	}
	return 0, false
}

func (s *State) applyPlayer(slot int, key, value string) error {
	p := &s.Players[slot]
	p.Named = true

	number := func(dst *int) error {
		n, err := strconv.Atoi(strings.TrimSpace(value))
		if err != nil {
			return fmt.Errorf("%s %q: %w", key, value, err)
		}
		*dst = n
		return nil
	}

	switch {
	// `play` and `battlefield` name the same zone; fixtures in the wild use
	// both.
	case strings.HasSuffix(key, "play"), strings.HasSuffix(key, "battlefield"):
		p.Battlefield = value
	case strings.HasSuffix(key, "hand"):
		p.Hand = value
	case strings.HasSuffix(key, "graveyard"):
		p.Graveyard = value
	case strings.HasSuffix(key, "library"):
		p.Library = value
	case strings.HasSuffix(key, "exile"):
		p.Exile = value
	case strings.HasSuffix(key, "command"):
		p.Command = value
	case strings.HasSuffix(key, "sideboard"):
		p.Sideboard = value
	case strings.HasSuffix(key, "manapool"):
		p.ManaPool = value
	case strings.HasSuffix(key, "persistentmana"):
		p.PersistentMana = value
	case strings.HasSuffix(key, "precast"):
		p.PrecastSpells = value
	case strings.HasSuffix(key, "putonstack"):
		p.PutOnStack = value
	case strings.HasSuffix(key, "counters"):
		p.Counters = value
	case strings.HasSuffix(key, "landsplayedlastturn"):
		return number(&p.LandsPlayedLastTurn)
	case strings.HasSuffix(key, "landsplayed"):
		return number(&p.LandsPlayed)
	case strings.HasSuffix(key, "life"):
		return number(&p.Life)
	case strings.HasSuffix(key, "numringtemptedyou"):
		return number(&p.NumRingTemptedYou)
	case strings.HasSuffix(key, "speed"):
		return number(&p.Speed)
	default:
		s.Unknown = append(s.Unknown, key)
	}
	return nil
}

// playerSlot maps a key's prefix to a player slot.
//
// `p` takes one digit and one only, which is Java's
// Integer.parseInt(String.valueOf(key.charAt(1))). So `p10life` is player 1
// with the category `0life`, not player 10 -- a quirk reproduced because a
// fixture written that way has to load the same way in both engines (PORT-7).
func playerSlot(key string) (int, bool) {
	switch {
	case strings.HasPrefix(key, "human"):
		return 0, true
	case strings.HasPrefix(key, "ai"):
		return 1, true
	case len(key) >= 2 && key[0] == 'p' && key[1] >= '0' && key[1] <= '9':
		return int(key[1] - '0'), true
	}
	return 0, false
}
