// Package keyword parses the value of a `K:` line.
//
// `Flying` is a keyword with nothing else to say; `Ward:2` carries an amount,
// `Dash:4 R W` a cost, and `Awaken:3:4 U` both. The arguments are positional
// and their meaning depends entirely on which keyword it is, so this package
// pairs a written line with the entry that says how to read it.
//
// Expanding a keyword into the triggers, statics and abilities it stands for
// is a separate job (ADR-0007 puts it at compile time rather than card
// construction), and it needs the effect layer that M3 is still building.
//
// Ported from forge-game/src/main/java/forge/game/keyword/Keyword.java
// (getKeywordDetails, smartValueOf) and the KeywordInstance subclasses that
// parse the details.
// Deviations recorded in docs/crucible/porting/port-log/keywords.md.
package keyword

import "strings"

// Kind is how a keyword's arguments are shaped, taken from the class Java's
// enum names for it.
type Kind uint8

// The argument shapes. The first six are Java's shared classes; Special covers
// the seventeen keywords with a parser of their own -- `Ward`, `Protection`,
// `Landwalk`, `Kicker`, `Suspend` and the rest -- whose details this package
// keeps as text.
const (
	// Unknown is a head Forge's enum does not define. It is not an error.
	Unknown Kind = iota
	Simple
	Amount
	Cost
	Type
	CostAndAmount
	CostAndType
	Special
)

var kindNames = [...]string{
	Unknown:       "Unknown",
	Simple:        "Simple",
	Amount:        "Amount",
	Cost:          "Cost",
	Type:          "Type",
	CostAndAmount: "CostAndAmount",
	CostAndType:   "CostAndType",
	Special:       "Special",
}

// String returns the kind's name.
func (k Kind) String() string { return kindNames[k] }

// Entry is one row of Forge's keyword enum.
type Entry struct {
	// Name is the display name, which is also what a script writes.
	Name string
	// Kind is the argument shape.
	Kind Kind
	// Class is the Java class that parses the details, kept because it is the
	// precise contract and seventeen of them are one-offs.
	Class string
	// MultipleRedundant says a second copy of the keyword does nothing.
	MultipleRedundant bool
}

// Keyword is one parsed `K:` line.
type Keyword struct {
	// Entry is the definition this line resolved to. Nil when the head names
	// no keyword Forge defines.
	Entry *Entry
	// Name is the head as written.
	Name string
	// Details is everything after the head, with a `:Flavor ` suffix removed.
	Details string
	// Text is the line exactly as the script wrote it.
	Text string
}

// Kind returns the argument shape, [Unknown] when the head names no keyword.
func (k Keyword) Kind() Kind {
	if k.Entry == nil {
		return Unknown
	}
	return k.Entry.Kind
}

// Args splits the details positionally, which is what every parser in the
// keyword package does with them.
func (k Keyword) Args() []string {
	if k.Details == "" {
		return nil
	}
	return strings.Split(k.Details, ":")
}

// Parse reads a `K:` value, reproducing Keyword.getKeywordDetails.
//
// Three cases, in Java's order:
//
//   - A `:` splits head from details at the first colon.
//   - Otherwise a space means the whole line may be a keyword whose name has
//     one -- `First strike`, `Cumulative upkeep`. Only if that fails is the
//     first word taken as the head.
//   - Otherwise the whole line is the head.
//
// A head naming nothing is not an error. `getInstance` falls back to a simple
// keyword holding the original text, which is how a card carries a line of
// rules text as a keyword, and 26 corpus heads do exactly that.
func Parse(line string) Keyword {
	out := Keyword{Text: line, Name: line}

	if head, details, ok := strings.Cut(line, ":"); ok {
		out.Name, out.Details = head, details
		// A flavour title is written last on the line and is display only.
		// Java cuts it off before the details are parsed so it cannot confuse
		// them.
		if at := strings.Index(out.Details, ":Flavor "); at >= 0 {
			out.Details = out.Details[:at]
		}
		out.Entry = Lookup(head)
		return out
	}

	if strings.Contains(line, " ") {
		if entry := Lookup(line); entry != nil {
			out.Entry = entry
			return out
		}
		head, details, _ := strings.Cut(line, " ")
		if entry := Lookup(head); entry != nil {
			out.Entry, out.Name, out.Details = entry, head, details
		}
		return out
	}

	out.Entry = Lookup(line)
	return out
}

// Lookup finds a keyword by name, ignoring case, which is smartValueOf.
func Lookup(name string) *Entry {
	for i := range Defined {
		if strings.EqualFold(Defined[i].Name, name) {
			return &Defined[i]
		}
	}
	return nil
}

// String writes the line back as it was read.
func (k Keyword) String() string { return k.Text }
