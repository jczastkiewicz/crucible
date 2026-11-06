package deck

import (
	"fmt"
	"strconv"
	"strings"
)

// Gauntlet is a gauntlet.toml manifest (docs/crucible/research/meta-gauntlet.md):
// what a format's decklists are, where they came from, and how much of the
// metagame each stands for.
type Gauntlet struct {
	Format   string
	Snapshot string
	Source   string
	Decks    []GauntletDeck
}

// GauntletDeck is one [[deck]] table. Weight is the archetype's metagame
// share as published; a reader normalises over the decks it uses.
type GauntletDeck struct {
	File      string
	Archetype string
	Weight    float64
	Event     string
	Date      string
	URL       string
}

// ParseGauntlet reads a gauntlet.toml: the TOML subset the manifest needs,
// `key = "string"` and `key = number` lines, `[[deck]]` tables and `#`
// comments. Anything else -- an unknown key or table, a key given twice, a
// deck without a file -- is an error, so a typo cannot silently drop a weight.
func ParseGauntlet(content []byte) (Gauntlet, error) {
	var g Gauntlet
	var deck *GauntletDeck
	seen := map[string]bool{}
	for i, raw := range strings.Split(string(content), "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "[") {
			if line != "[[deck]]" {
				return Gauntlet{}, fmt.Errorf("gauntlet.toml:%d: unknown table %s", i+1, line)
			}
			g.Decks = append(g.Decks, GauntletDeck{})
			deck = &g.Decks[len(g.Decks)-1]
			seen = map[string]bool{}
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			return Gauntlet{}, fmt.Errorf("gauntlet.toml:%d: %q is not key = value", i+1, line)
		}
		key = strings.TrimSpace(key)
		if seen[key] {
			return Gauntlet{}, fmt.Errorf("gauntlet.toml:%d: %s given twice", i+1, key)
		}
		seen[key] = true
		if err := g.set(deck, key, strings.TrimSpace(value)); err != nil {
			return Gauntlet{}, fmt.Errorf("gauntlet.toml:%d: %w", i+1, err)
		}
	}
	if g.Format == "" || g.Snapshot == "" || g.Source == "" {
		return Gauntlet{}, fmt.Errorf("gauntlet.toml: format, snapshot and source are required")
	}
	for i, d := range g.Decks {
		if d.File == "" {
			return Gauntlet{}, fmt.Errorf("gauntlet.toml: deck %d has no file", i+1)
		}
	}
	return g, nil
}

// set assigns one key of the top level (deck nil) or of the current deck.
func (g *Gauntlet) set(deck *GauntletDeck, key, value string) error {
	if deck != nil && key == "weight" {
		w, err := strconv.ParseFloat(value, 64)
		if err != nil || w < 0 {
			return fmt.Errorf("weight %q is not a non-negative number", value)
		}
		deck.Weight = w
		return nil
	}
	s, err := tomlString(value)
	if err != nil {
		return fmt.Errorf("%s: %w", key, err)
	}
	var field *string
	if deck == nil {
		field = map[string]*string{"format": &g.Format, "snapshot": &g.Snapshot, "source": &g.Source}[key]
	} else {
		field = map[string]*string{"file": &deck.File, "archetype": &deck.Archetype, "event": &deck.Event,
			"date": &deck.Date, "url": &deck.URL}[key]
	}
	if field == nil {
		return fmt.Errorf("unknown key %s", key)
	}
	*field = s
	return nil
}

// tomlString reads a TOML basic string: double quotes, with \" and \\ the
// only escapes the manifest uses.
func tomlString(value string) (string, error) {
	if len(value) < 2 || value[0] != '"' || value[len(value)-1] != '"' {
		return "", fmt.Errorf("%s is not a quoted string", value)
	}
	var b strings.Builder
	body := value[1 : len(value)-1]
	for i := 0; i < len(body); i++ {
		c := body[i]
		switch {
		case c == '\\' && i+1 < len(body) && (body[i+1] == '"' || body[i+1] == '\\'):
			b.WriteByte(body[i+1])
			i++
		case c == '\\' || c == '"':
			return "", fmt.Errorf("%s: unsupported escape or stray quote", value)
		default:
			b.WriteByte(c)
		}
	}
	return b.String(), nil
}
