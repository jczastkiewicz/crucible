package fixture_test

import (
	"strings"
	"testing"

	"github.com/jczastkiewicz/crucible/internal/carddb"
	"github.com/jczastkiewicz/crucible/internal/carddb/compile"
	"github.com/jczastkiewicz/crucible/internal/cardtype"
	"github.com/jczastkiewicz/crucible/internal/fixture"
)

// typedDB is testDB with a type line per card, for the dump gates that read
// the type (Card.isSick).
func typedDB(t *testing.T, types map[string]string) *compile.DB {
	t.Helper()

	cards := make(map[string]*compile.Card, len(types))
	for name, line := range types {
		raw := &carddb.Card{Filename: name}
		raw.Faces[0] = carddb.Face{Present: true, Name: name, Type: cardtype.ParseToken(line)}
		c, err := compile.Compile(raw)
		if err != nil {
			t.Fatalf("compile %q: %v", name, err)
		}
		cards[name] = c
	}
	return compile.NewDB(cards)
}

// |SummonSick is written by Card.isSick (GameState.java:323-325): a creature
// with sickness writes it, a noncreature with the same flag writes none.
func TestSummonSickDumpsOnlyASickCreature(t *testing.T) {
	t.Parallel()

	l := load(t, typedDB(t, map[string]string{"Grizzly Bears": "Creature", "Sol Ring": "Artifact"}), "humanlife=20\nailife=20\nhumanbattlefield=Grizzly Bears|SummonSick;Sol Ring|SummonSick\n")
	bf := fixture.Dump(l).Players[0].Battlefield
	entries := strings.Split(bf, ";")
	if len(entries) != 2 {
		t.Fatalf("battlefield %q, want two entries", bf)
	}
	if !strings.Contains(entries[0], "|SummonSick") {
		t.Errorf("%q lacks |SummonSick", entries[0])
	}
	if strings.Contains(entries[1], "SummonSick") {
		t.Errorf("%q carries |SummonSick, want none on a noncreature", entries[1])
	}
}

// |Renowned and |Monstrous load into the card (GameState.java:1321-1330) and
// dump back (:326-337).
func TestRenownedAndMonstrousRoundTrip(t *testing.T) {
	t.Parallel()

	l := load(t, testDB(t, "Grizzly Bears"), "humanlife=20\nailife=20\nhumanbattlefield=Grizzly Bears|Id:1|Renowned;Grizzly Bears|Id:2|Monstrous\n")
	if len(l.Unapplied) != 0 {
		t.Errorf("Unapplied = %v, want none", l.Unapplied)
	}
	c1, c2 := l.Game.Card(l.CardByFixtureID[1]), l.Game.Card(l.CardByFixtureID[2])
	if !c1.IsRenowned() || c1.IsMonstrous() {
		t.Errorf("card 1 renowned=%v monstrous=%v, want renowned only", c1.IsRenowned(), c1.IsMonstrous())
	}
	if c2.IsRenowned() || !c2.IsMonstrous() {
		t.Errorf("card 2 renowned=%v monstrous=%v, want monstrous only", c2.IsRenowned(), c2.IsMonstrous())
	}
	bf := fixture.Dump(l).Players[0].Battlefield
	if !strings.Contains(bf, "|Renowned") || !strings.Contains(bf, "|Monstrous") {
		t.Errorf("battlefield %q lacks |Renowned or |Monstrous", bf)
	}
}

// A transformed card dumps under its paper card's name, the front face's,
// then |Transformed (GameState.java writes getPaperCard().getName()), and
// loads back transformed.
func TestTransformedDumpsTheFrontFaceName(t *testing.T) {
	t.Parallel()

	raw := &carddb.Card{Filename: "test_wolf", SplitType: carddb.SplitTransform}
	raw.Faces[carddb.FacePrimary] = carddb.Face{Present: true, Name: "Test Shepherd", Type: cardtype.ParseToken("Creature")}
	raw.Faces[carddb.FaceAlternate] = carddb.Face{Present: true, Name: "Test Howler", Type: cardtype.ParseToken("Creature")}
	c, err := compile.Compile(raw)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	db := compile.NewDB(map[string]*compile.Card{"Test Shepherd": c})

	l := load(t, db, "humanlife=20\nailife=20\nhumanbattlefield=Test Shepherd|Transformed\n")
	bf := fixture.Dump(l).Players[0].Battlefield
	if !strings.HasPrefix(bf, "Test Shepherd|") || !strings.Contains(bf, "|Transformed") {
		t.Fatalf("battlefield %q, want Test Shepherd ... |Transformed", bf)
	}
	again := load(t, db, "humanlife=20\nailife=20\nhumanbattlefield="+bf+"\n")
	if got := fixture.Dump(again).Players[0].Battlefield; got != bf {
		t.Errorf("reloaded battlefield %q, want %q", got, bf)
	}
}
