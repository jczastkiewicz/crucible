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

// TestIsPureMana covers the one predicate resolveUnlessCost (internal/engine)
// still uses directly -- a genuinely different question from
// [cost.Cost.ActivationShape], below: an "unless a cost is paid" cost never
// allows Tap/SelfSac/Discard the way an activation cost does.
func TestIsPureMana(t *testing.T) {
	t.Parallel()

	cases := []struct {
		text     string
		pureMana bool
	}{
		{"", true},
		{"1 U", true},
		{"B B", true},
		{"T", false},
		{"1 U T", false},
		{"Q", false},
		{"Untap", false},
		{"Sac<1/Creature>", false},
		{"1 U Sac<1/Creature>", false},
		{"Mandatory PayEnergy<2>", false},
		{"XMin2", false},
	}
	for _, c := range cases {
		got := cost.Parse(c.text)
		if got.IsPureMana() != c.pureMana {
			t.Errorf("Parse(%q).IsPureMana() = %v, want %v", c.text, got.IsPureMana(), c.pureMana)
		}
	}
}

// TestActivationShape covers Tap, SelfSac, Discard and PayLife both alone and
// combined -- the shape ActivateAbility/ActivateManaAbility (internal/engine)
// actually pay.
func TestActivationShape(t *testing.T) {
	t.Parallel()

	cases := []struct {
		text   string
		want   cost.ActivationShape
		wantOK bool
	}{
		{"", cost.ActivationShape{}, true},
		{"1 U", cost.ActivationShape{}, true},
		{"T", cost.ActivationShape{Tap: true}, true},
		{"Sac<1/CARDNAME>", cost.ActivationShape{SelfSac: true}, true},
		{"2 G T Sac<1/CARDNAME>", cost.ActivationShape{Tap: true, SelfSac: true}, true},
		{"Discard<1/Card>", cost.ActivationShape{DiscardN: 1}, true},
		{"Discard<2/Card>", cost.ActivationShape{DiscardN: 2}, true},
		{"T Discard<1/Card>", cost.ActivationShape{Tap: true, DiscardN: 1}, true},
		{"Sac<1/CARDNAME> Discard<1/Card>", cost.ActivationShape{SelfSac: true, DiscardN: 1}, true},
		{"1 R T Sac<1/CARDNAME> Discard<2/Card>",
			cost.ActivationShape{Tap: true, SelfSac: true, DiscardN: 2}, true},
		{"PayLife<1>", cost.ActivationShape{PayLifeN: 1}, true},
		{"PayLife<2>", cost.ActivationShape{PayLifeN: 2}, true},
		{"1 PayLife<2>", cost.ActivationShape{PayLifeN: 2}, true},
		{"T PayLife<1>", cost.ActivationShape{Tap: true, PayLifeN: 1}, true},
		{"Sac<1/CARDNAME> PayLife<1>", cost.ActivationShape{SelfSac: true, PayLifeN: 1}, true},
		{"Discard<1/Card> PayLife<1>", cost.ActivationShape{DiscardN: 1, PayLifeN: 1}, true},
		{"1 T Sac<1/CARDNAME> Discard<1/Card> PayLife<2>",
			cost.ActivationShape{Tap: true, SelfSac: true, DiscardN: 1, PayLifeN: 2}, true},
		{"Sac<1/Creature.Other/another creature>", cost.ActivationShape{}, false},
		{"Sac<2/CARDNAME>", cost.ActivationShape{}, false},
		{"Sac<1/CARDNAME> Sac<1/CARDNAME>", cost.ActivationShape{}, false},
		{"Discard<1/CARDNAME>", cost.ActivationShape{}, false},
		{"Discard<0/Card>", cost.ActivationShape{}, false},
		{"Discard<1/Card> Discard<1/Card>", cost.ActivationShape{}, false},
		{"PayLife<0>", cost.ActivationShape{}, false},
		{"PayLife<X/half your life, rounded up>", cost.ActivationShape{}, false},
		{"PayLife<1> PayLife<1>", cost.ActivationShape{}, false},
		{"Untap Sac<1/CARDNAME>", cost.ActivationShape{}, false},
		{"Mandatory Sac<1/CARDNAME>", cost.ActivationShape{}, false},
		{"XMin1 Sac<1/CARDNAME>", cost.ActivationShape{}, false},
	}
	for _, c := range cases {
		got, ok := cost.Parse(c.text).ActivationShape()
		if ok != c.wantOK {
			t.Errorf("Parse(%q).ActivationShape() ok = %v, want %v", c.text, ok, c.wantOK)
			continue
		}
		if ok && got != c.want {
			t.Errorf("Parse(%q).ActivationShape() = %+v, want %+v", c.text, got, c.want)
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
