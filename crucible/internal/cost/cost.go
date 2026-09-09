// Package cost parses the value of a `Cost$` param: what a player pays to
// activate an ability or cast a spell.
//
// `1 R Sac<1/Creature.Other/another creature> T` is four parts — two mana
// symbols, a sacrifice and a tap. Paying one needs a game, so this package
// only says what the parts are.
//
// Ported from forge-game/src/main/java/forge/game/cost/Cost.java (its
// constructor, parseCostPart and abCostParse), with the field limits taken
// from the 51 branches of parseCostPart.
// Deviations recorded in docs/crucible/porting/port-log/cost-strings.md.
package cost

import "strings"

// Cost is a parsed cost string.
type Cost struct {
	// Parts are the named parts, in the order the string writes them.
	Parts []Part
	// Mana holds the tokens no named part claimed. Java appends them to a mana
	// string and hands that to the mana cost parser, so an unrecognised part is
	// silently mana rather than an error.
	Mana []string

	// Tap and Untap are `T`/`Tap` and `Q`/`Untap`. They are read in a pass of
	// their own before anything else, because two named parts take the flags as
	// constructor arguments.
	Tap   bool
	Untap bool
	// Mandatory is the `Mandatory` token, which makes the cost unskippable.
	Mandatory bool
	// XMin is an `XMin...` token, kept as written.
	XMin string

	// Text is the cost exactly as the script wrote it.
	Text string
}

// Part is one named cost part: a name and the fields of its `<...>` body.
type Part struct {
	// Name is the part's name, without the body.
	Name string
	// Fields are the `/`-separated body fields, split with this part's own
	// limit, so a description containing a slash keeps it.
	Fields []string
	// Text is the part as written, body included.
	Text string
}

// Field returns a body field by position, empty when the body has no such
// field. Java reads its optional fields exactly this way -- a length check and
// a default -- so the zero value is a real answer rather than a missing one.
func (p Part) Field(i int) string {
	if i < len(p.Fields) {
		return p.Fields[i]
	}
	return ""
}

// Parse reads a cost string.
//
// It never fails, because Java's constructor cannot: a token matching no named
// part becomes mana, and a malformed mana token is the mana parser's problem
// later.
func Parse(text string) Cost {
	out := Cost{Text: text}
	if strings.TrimSpace(text) == "" {
		return out
	}

	// Bracket-aware, because 12% of `<...>` bodies contain a space: splitting
	// on whitespace first would cut `Sac<1/Creature.Other/another creature>`
	// in half.
	tokens := split(text, ' ', maxEntries, '<', '>')

	// Java's own pre-pass. CostTapType and CostUntapType are constructed with
	// these flags, so they have to be known before any part is built.
	for _, token := range tokens {
		switch token {
		case "T", "Tap":
			out.Tap = true
		case "Q", "Untap":
			out.Untap = true
		}
	}

	for _, token := range tokens {
		switch {
		case strings.HasPrefix(token, "XMin"):
			out.XMin = token
		case token == "Mandatory":
			out.Mandatory = true
		default:
			if part, ok := parsePart(token); ok {
				out.Parts = append(out.Parts, part)
				continue
			}
			out.Mana = append(out.Mana, token)
		}
	}
	return out
}

// parsePart reads one token as a named cost part.
func parsePart(token string) (Part, bool) {
	for _, named := range namedParts {
		if named.exact {
			if token != named.prefix {
				continue
			}
		} else if !strings.HasPrefix(token, named.prefix) {
			continue
		}
		part := Part{Name: named.name, Text: token}
		if named.fields > 0 {
			part.Fields = split(bodyOf(token), '/', named.fields, 0, 0)
		}
		return part, true
	}
	return Part{}, false
}

// bodyOf returns what sits between the first `<` and the next `>`, which is
// abCostParse's substring.
func bodyOf(token string) string {
	open := strings.IndexByte(token, '<')
	if open < 0 {
		return ""
	}
	end := strings.IndexByte(token[open+1:], '>')
	if end < 0 {
		return token[open+1:]
	}
	return token[open+1 : open+1+end]
}

// String writes the cost back as it was read.
func (c Cost) String() string { return c.Text }

// maxEntries is Java's Integer.MAX_VALUE, meaning no cap.
const maxEntries = int(^uint(0) >> 1)
