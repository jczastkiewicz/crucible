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

// IsPureMana reports whether the cost is nothing but mana symbols -- no
// named Part, no Tap/Untap/Mandatory token, and no XMin. A caller that can
// only pay mana (Game.PayManaCost and its own callers) uses this to decide
// whether it can attempt payment at all, rather than reading every other
// field itself.
func (c Cost) IsPureMana() bool {
	return len(c.Parts) == 0 && !c.Tap && !c.Untap && !c.Mandatory && c.XMin == ""
}

// IsPureManaOrTap reports whether the cost is nothing but mana symbols and,
// optionally, a single Tap-self token ("T"/"Tap") -- CR 602's own dominant
// real activation cost shape. Parts always carries a lone Name "T" entry
// alongside Tap itself once a script writes "T" (namedParts' own trailing
// {name: "T", prefix: "T", exact: true} branch, parseCostPart's own
// redundant-looking second read of the identical token this package's own
// Tap flag already carries -- Java keeps both because CostPartTap is a real
// CostPart object, described and iterated like any other, not only a
// boolean), so this cannot reuse IsPureMana's own flat "no Parts at all"
// contract: it allows exactly that one entry and rejects any other.
func (c Cost) IsPureManaOrTap() bool {
	if c.Untap || c.Mandatory || c.XMin != "" {
		return false
	}
	for _, p := range c.Parts {
		if p.Name != "T" {
			return false
		}
	}
	return true
}

// IsPureManaTapAndSelfSac reports whether the cost is nothing but mana
// symbols, an optional Tap-self token, and an optional self-sacrifice Part
// (Sac<1/CARDNAME>) -- IsPureManaOrTap's own sibling, for CR 602's own
// second-most-common real activation cost shape past bare mana/Tap: "sac
// CARDNAME" as part of an activation cost (fetch lands, sac outlets, Treasure
// tokens' own AB$ Mana line among them, though that API is out of this
// predicate's own caller's scope). Reuses IsPureManaOrTap's own
// Untap/Mandatory/XMin rejection, then allows exactly one further Part
// naming Sac<1/CARDNAME> -- SelfSac's own doc comment has the exact shape --
// alongside the lone "T" Part IsPureManaOrTap already allows.
func (c Cost) IsPureManaTapAndSelfSac() bool {
	if c.Untap || c.Mandatory || c.XMin != "" {
		return false
	}
	sacSeen := false
	for _, p := range c.Parts {
		switch {
		case p.Name == "T":
		case p.Name == "Sac" && !sacSeen && p.Field(0) == "1" && p.Field(1) == "CARDNAME":
			sacSeen = true
		default:
			return false
		}
	}
	return true
}

// SelfSac reports whether the cost is IsPureManaTapAndSelfSac's own shape
// AND actually carries the Sac<1/CARDNAME> Part -- "sacrifice this
// permanent" written with the literal self-reference token CARDNAME, the
// only Sac<...> shape that predicate ever admits. Answers false on its own
// for a cost IsPureManaTapAndSelfSac would reject too (an extra Part, a
// chosen or SVar-sized Sac<...>, ...), rather than only being a safe question
// once a caller has checked that predicate first.
func (c Cost) SelfSac() bool {
	if !c.IsPureManaTapAndSelfSac() {
		return false
	}
	for _, p := range c.Parts {
		if p.Name == "Sac" {
			return true
		}
	}
	return false
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
