package cost_test

import (
	"strings"
	"testing"

	"github.com/jczastkiewicz/crucible/internal/cost"
)

// The finding that sizes the parser: 12% of `<...>` bodies contain a space, so
// splitting on whitespace first cuts a cost part in half.
func TestBracketedBodiesStayWhole(t *testing.T) {
	t.Parallel()

	c := cost.Parse("1 B Sac<1/Creature.Other/another creature> T")
	if diff := diffStrings(c.Mana, []string{"1", "B"}); diff != "" {
		t.Errorf("mana: %s", diff)
	}
	if len(c.Parts) != 2 {
		t.Fatalf("parsed %d parts, want 2: %+v", len(c.Parts), c.Parts)
	}
	if c.Parts[0].Name != "Sac" || c.Parts[0].Field(2) != "another creature" {
		t.Errorf("part = %+v, want Sac with its description intact", c.Parts[0])
	}
	if !c.Tap {
		t.Error("Tap is false, want true")
	}
}

// The field limit is Java's own, per part, so a description keeps a slash
// instead of becoming another field.
func TestFieldLimits(t *testing.T) {
	t.Parallel()

	c := cost.Parse("tapXType<2/Creature;Treasure/creatures and/or Treasures>")
	if len(c.Parts) != 1 {
		t.Fatalf("parsed %d parts, want 1", len(c.Parts))
	}
	part := c.Parts[0]
	if got := len(part.Fields); got != 3 {
		t.Fatalf("body split into %d fields, want 3: %q", got, part.Fields)
	}
	if part.Field(2) != "creatures and/or Treasures" {
		t.Errorf("description = %q, want the slash kept", part.Field(2))
	}
}

// SubCounter takes five fields, and the zone list is the fifth.
func TestSubCounterFields(t *testing.T) {
	t.Parallel()

	c := cost.Parse("1 R SubCounter<1/TIME/Permanent.inZoneBattlefield;Card.suspended/a permanent you control or suspended card you own/Battlefield,Exile>")
	part := c.Parts[0]
	if part.Name != "SubCounter" || len(part.Fields) != 5 {
		t.Fatalf("part = %+v, want SubCounter with five fields", part)
	}
	if part.Field(4) != "Battlefield,Exile" {
		t.Errorf("zones = %q, want %q", part.Field(4), "Battlefield,Exile")
	}
}

// A field the body does not have reads as empty, which is how Java reads its
// optional fields: a length check and a default.
func TestMissingFieldsAreEmpty(t *testing.T) {
	t.Parallel()

	part := cost.Parse("Sac<1/Creature>").Parts[0]
	if part.Field(1) != "Creature" || part.Field(2) != "" || part.Field(9) != "" {
		t.Errorf("fields = %q, want an empty answer past the end", part.Fields)
	}
}

// Anything no named part claims is mana. Java appends it to a mana string and
// hands that to the mana parser, so an unknown token is silently a cost rather
// than an error.
func TestUnknownTokensAreMana(t *testing.T) {
	t.Parallel()

	c := cost.Parse("2 W W Wibble")
	if diff := diffStrings(c.Mana, []string{"2", "W", "W", "Wibble"}); diff != "" {
		t.Errorf("mana: %s", diff)
	}
	if len(c.Parts) != 0 {
		t.Errorf("parts = %+v, want none", c.Parts)
	}
}

// `T` is the tap cost and `Teamwork<...>` is not: Java tests equals for the
// bare parts and startsWith for the rest.
func TestBarePartsMatchExactly(t *testing.T) {
	t.Parallel()

	c := cost.Parse("T Teamwork<1> Forage Q")
	var names []string
	for _, p := range c.Parts {
		names = append(names, p.Name)
	}
	if diff := diffStrings(names, []string{"T", "Teamwork", "Forage", "Q"}); diff != "" {
		t.Errorf("parts: %s -- script order, as Java builds them", diff)
	}
	if !c.Tap || !c.Untap {
		t.Errorf("Tap = %v, Untap = %v; want both", c.Tap, c.Untap)
	}
}

func TestFlagsAndXMin(t *testing.T) {
	t.Parallel()

	c := cost.Parse("XMin1 Mandatory PayLife<3>")
	if c.XMin != "XMin1" {
		t.Errorf("XMin = %q, want XMin1", c.XMin)
	}
	if !c.Mandatory {
		t.Error("Mandatory is false, want true")
	}
	if len(c.Parts) != 1 || c.Parts[0].Name != "PayLife" {
		t.Errorf("parts = %+v, want one PayLife", c.Parts)
	}
}

// Two delimiters in a row collapse, because Java's splitter skips empty
// segments -- which means an empty field shifts every field after it.
func TestEmptySegmentsCollapse(t *testing.T) {
	t.Parallel()

	c := cost.Parse("1  R")
	if diff := diffStrings(c.Mana, []string{"1", "R"}); diff != "" {
		t.Errorf("mana: %s", diff)
	}

	part := cost.Parse("Sac<1//a creature>").Parts[0]
	if diff := diffStrings(part.Fields, []string{"1", "a creature"}); diff != "" {
		t.Errorf("fields: %s -- the empty field is dropped, not kept", diff)
	}
}

func TestRoundTrip(t *testing.T) {
	t.Parallel()

	for _, text := range []string{
		"T",
		"1 R",
		"Sac<1/Creature.Other/another creature>",
		"1 B Sac<1/Artifact;Creature/artifact or creature>",
		"tapXType<2/Creature;Treasure/creatures and/or Treasures>",
	} {
		if got := cost.Parse(text).String(); got != text {
			t.Errorf("Parse(%q).String() = %q", text, got)
		}
	}
}

func TestEmpty(t *testing.T) {
	t.Parallel()

	c := cost.Parse("")
	if len(c.Parts) != 0 || len(c.Mana) != 0 {
		t.Errorf("Parse(\"\") = %+v, want nothing", c)
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
