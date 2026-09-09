package keyword_test

import (
	"strings"
	"testing"

	"github.com/jczastkiewicz/crucible/internal/keyword"
)

// Three cases in Java's order: a colon splits, then a space may be part of the
// name, and only then is the first word the head.
func TestParseCases(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		line    string
		name    string
		details string
		kind    keyword.Kind
	}{
		{"Flying", "Flying", "", keyword.Simple},
		{"First strike", "First strike", "", keyword.Simple},
		{"Cumulative upkeep:2", "Cumulative upkeep", "2", keyword.Cost},
		{"Ward:2", "Ward", "2", keyword.Special},
		{"Dash:4 R W", "Dash", "4 R W", keyword.Cost},
		{"Awaken:3:4 U", "Awaken", "3:4 U", keyword.CostAndAmount},
		{"Absorb:1", "Absorb", "1", keyword.Amount},
		{"Champion:Elf", "Champion", "Elf", keyword.Type},
	} {
		got := keyword.Parse(tt.line)
		if got.Name != tt.name || got.Details != tt.details {
			t.Errorf("Parse(%q) = %q / %q, want %q / %q", tt.line, got.Name, got.Details, tt.name, tt.details)
		}
		if got.Kind() != tt.kind {
			t.Errorf("Parse(%q).Kind() = %s, want %s", tt.line, got.Kind(), tt.kind)
		}
	}
}

// A name is matched without case, which is smartValueOf.
func TestLookupIgnoresCase(t *testing.T) {
	t.Parallel()

	for _, name := range []string{"flying", "FLYING", "Flying", "first strike"} {
		if keyword.Lookup(name) == nil {
			t.Errorf("Lookup(%q) found nothing", name)
		}
	}
	if keyword.Lookup("Wibble") != nil {
		t.Error("Lookup found a keyword that does not exist")
	}
}

// A flavour title is display only and is cut before the details are read, so a
// keyword that takes an amount still sees only the amount.
func TestFlavorSuffixIsCut(t *testing.T) {
	t.Parallel()

	got := keyword.Parse("Ward:2:Flavor Protective Ward")
	if got.Details != "2" {
		t.Errorf("details = %q, want %q", got.Details, "2")
	}
	if got.Text != "Ward:2:Flavor Protective Ward" {
		t.Errorf("text = %q, want the line unchanged", got.Text)
	}
}

// A head naming no keyword is not an error: Forge falls back to a simple
// keyword holding the text, which is how a card carries plain rules text.
func TestUnknownHeadIsNotAnError(t *testing.T) {
	t.Parallel()

	for _, line := range []string{"etbCounter:P1P1:1", "CARDNAME can't be blocked."} {
		got := keyword.Parse(line)
		if got.Kind() != keyword.Unknown || got.Entry != nil {
			t.Errorf("Parse(%q) resolved to %+v, want Unknown", line, got.Entry)
		}
		if got.Text != line {
			t.Errorf("text = %q, want the line unchanged", got.Text)
		}
	}
}

// The details are positional, so a caller reads them by index and the meaning
// comes from the keyword rather than from the shape.
func TestArgs(t *testing.T) {
	t.Parallel()

	if diff := diffStrings(keyword.Parse("Awaken:3:4 U").Args(), []string{"3", "4 U"}); diff != "" {
		t.Errorf("Awaken: %s", diff)
	}
	if diff := diffStrings(keyword.Parse("Flying").Args(), nil); diff != "" {
		t.Errorf("Flying: %s", diff)
	}
	if diff := diffStrings(keyword.Parse("Dash:4 R W").Args(), []string{"4 R W"}); diff != "" {
		t.Errorf("Dash: %s -- a cost keeps its spaces", diff)
	}
}

// The table is Java's enum, transcribed. Its size is the claim worth pinning:
// a keyword added upstream has to arrive here too.
func TestDefinedTable(t *testing.T) {
	t.Parallel()

	if got := len(keyword.Defined); got != 202 {
		t.Errorf("Defined holds %d keywords, want 202 (203 enum constants less UNDEFINED)", got)
	}

	kinds := map[keyword.Kind]int{}
	for _, entry := range keyword.Defined {
		kinds[entry.Kind]++
		if entry.Name == "" || entry.Class == "" {
			t.Errorf("entry %+v is incomplete", entry)
		}
	}
	// 88 + 54 + 27 + 5 + 3 + 2 + 23 = 202. UNDEFINED is a SimpleKeyword with no
	// name and is not a keyword, so it is not in the table.
	for kind, want := range map[keyword.Kind]int{
		keyword.Simple:        88,
		keyword.Cost:          54,
		keyword.Amount:        27,
		keyword.Type:          5,
		keyword.CostAndAmount: 3,
		keyword.CostAndType:   2,
		keyword.Special:       23,
	} {
		if got := kinds[kind]; got != want {
			t.Errorf("%s keywords: %d, want %d", kind, got, want)
		}
	}
}

func diffStrings(got, want []string) string {
	if len(got) == len(want) {
		same := true
		for i := range got {
			if got[i] != want[i] {
				same = false
				break
			}
		}
		if same {
			return ""
		}
	}
	return "got [" + strings.Join(got, " ") + "], want [" + strings.Join(want, " ") + "]"
}
