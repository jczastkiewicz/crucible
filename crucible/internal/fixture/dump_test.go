package fixture_test

import (
	"strings"
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
	"github.com/jczastkiewicz/crucible/internal/fixture"
	"github.com/jczastkiewicz/crucible/pkg/javarand"
)

// TestDumpRoundTripsThroughParse is the P3 exit gate itself: a scenario
// fixture has to load, dump, and load again to the same game -- Parse ->
// Load -> Dump -> Write -> Parse -> Load, checked field by field.
func TestDumpRoundTripsThroughParse(t *testing.T) {
	t.Parallel()

	db := testDB(t, "Rancor", "Grizzly Bears", "Llanowar Elves", "Mountain")
	original := "humanlife=17\nailife=9\nturn=5\nactiveplayer=ai\nactivephase=Main2\n" +
		"humanbattlefield=Rancor|Id:1|AttachedTo:2|Tapped;Grizzly Bears|Id:2|Counters:P1P1=2|Damage:1|SummonSick\n" +
		"humanhand=Mountain;Llanowar Elves\n" +
		"ailife=9\n"

	l1 := load(t, db, original)
	st2 := fixture.Dump(l1)

	var buf strings.Builder
	if err := fixture.Write(&buf, st2); err != nil {
		t.Fatalf("Write: %v", err)
	}

	st3, err := fixture.Parse(strings.NewReader(buf.String()))
	if err != nil {
		t.Fatalf("Parse(Write(Dump(x))) failed: %v\ntext:\n%s", err, buf.String())
	}
	l2, err := fixture.Load(st3, db, javarand.New(1))
	if err != nil {
		t.Fatalf("Load(Parse(Write(Dump(x)))) failed: %v\ntext:\n%s", err, buf.String())
	}

	// Life, turn and phase survive.
	if l2.Game.Turn() != l1.Game.Turn() {
		t.Errorf("turn %d, want %d", l2.Game.Turn(), l1.Game.Turn())
	}
	if got, want := l2.Game.Player(l2.Game.ActivePlayer()).Name, l1.Game.Player(l1.Game.ActivePlayer()).Name; got != want {
		t.Errorf("active player %q, want %q", got, want)
	}
	if l2.Game.ActivePhase() != l1.Game.ActivePhase() {
		t.Errorf("active phase %v, want %v", l2.Game.ActivePhase(), l1.Game.ActivePhase())
	}

	human1, human2 := l1.Game.Players()[0], l2.Game.Players()[0]
	if got, want := l2.Game.Player(human2).Life, l1.Game.Player(human1).Life; got != want {
		t.Errorf("human life %d, want %d", got, want)
	}

	bf1 := l1.Game.Zone(engine.Battlefield, human1).Cards()
	bf2 := l2.Game.Zone(engine.Battlefield, human2).Cards()
	if len(bf1) != len(bf2) {
		t.Fatalf("battlefield has %d cards, want %d", len(bf2), len(bf1))
	}
	for i := range bf1 {
		c1, c2 := l1.Game.Card(bf1[i]), l2.Game.Card(bf2[i])
		if c1.Def.Name != c2.Def.Name {
			t.Errorf("card %d name %q, want %q", i, c2.Def.Name, c1.Def.Name)
		}
		if c1.Tapped != c2.Tapped {
			t.Errorf("%s tapped %v, want %v", c1.Def.Name, c2.Tapped, c1.Tapped)
		}
		if c1.SummonSick != c2.SummonSick {
			t.Errorf("%s summonsick %v, want %v", c1.Def.Name, c2.SummonSick, c1.SummonSick)
		}
		if c1.Damage.Marked != c2.Damage.Marked {
			t.Errorf("%s damage %d, want %d", c1.Def.Name, c2.Damage.Marked, c1.Damage.Marked)
		}
		if c1.Counters.Count(engine.P1P1) != c2.Counters.Count(engine.P1P1) {
			t.Errorf("%s P1P1 %d, want %d", c1.Def.Name, c2.Counters.Count(engine.P1P1), c1.Counters.Count(engine.P1P1))
		}
	}

	// The attachment survived the round trip even though the fixture's own
	// Id numbers were replaced by Dump with fresh CardIDs.
	rancor2 := bf2[0]
	bears2 := bf2[1]
	host, ok := l2.Game.Card(rancor2).AttachedTo()
	if !ok || host != bears2 {
		t.Errorf("Rancor attached to %v ok=%v after round trip, want Grizzly Bears", host, ok)
	}

	hand1 := l1.Game.Zone(engine.Hand, human1).Cards()
	hand2 := l2.Game.Zone(engine.Hand, human2).Cards()
	if len(hand1) != len(hand2) {
		t.Fatalf("hand has %d cards, want %d", len(hand2), len(hand1))
	}
	for i := range hand1 {
		if got, want := l2.Game.Card(hand2[i]).Def.Name, l1.Game.Card(hand1[i]).Def.Name; got != want {
			t.Errorf("hand card %d %q, want %q", i, got, want)
		}
	}
}

// Dump round-trips Lost/Won/Over through Write and back, the way it already
// does for Life and Counters -- the whole point of adding them was to make
// this comparable at all (game-state-fixture.md).
func TestDumpAndWriteRoundTripLostWonOver(t *testing.T) {
	t.Parallel()

	db := testDB(t)
	l := load(t, db, "humanlife=0\nhumanlost=true\nailife=20\naiwon=true\nover=true\n")
	st := fixture.Dump(l)

	if !st.Players[0].Lost {
		t.Error("Dump: human.Lost did not carry over")
	}
	if !st.Players[1].Won {
		t.Error("Dump: ai.Won did not carry over")
	}
	if !st.Over {
		t.Error("Dump: Over did not carry over")
	}

	var buf strings.Builder
	if err := fixture.Write(&buf, st); err != nil {
		t.Fatalf("Write: %v", err)
	}
	got, err := fixture.Parse(strings.NewReader(buf.String()))
	if err != nil {
		t.Fatalf("Parse(Write(x)): %v", err)
	}
	if !got.Players[0].Lost || !got.Players[1].Won || !got.Over {
		t.Errorf("round trip: lost=%v won=%v over=%v, want true/true/true",
			got.Players[0].Lost, got.Players[1].Won, got.Over)
	}
}

func TestDumpOwnerOnlyWhenDifferentFromController(t *testing.T) {
	t.Parallel()

	db := testDB(t, "Grizzly Bears", "Llanowar Elves")
	l := load(t, db, "humanbattlefield=Grizzly Bears|Owner:ai;Llanowar Elves\nailife=20\n")
	st := fixture.Dump(l)

	if !strings.Contains(st.Players[0].Battlefield, "Owner:ai") {
		t.Errorf("battlefield %q does not carry Owner:ai for the stolen card", st.Players[0].Battlefield)
	}
	entries := strings.Split(st.Players[0].Battlefield, ";")
	if strings.Contains(entries[1], "Owner:") {
		t.Errorf("second entry %q carries an Owner:, want none -- owner equals controller", entries[1])
	}
}

// Counters is allowed in Battlefield or Exile; Tapped is Battlefield only.
// Java's own writer gates these per zone, and Dump has to match it or the
// round trip would move state to a zone it never asked to represent.
func TestDumpZoneGating(t *testing.T) {
	t.Parallel()

	db := testDB(t, "Grizzly Bears")
	l := load(t, db, "humanexile=Grizzly Bears|Counters:P1P1=1|Tapped\n")
	st := fixture.Dump(l)

	if !strings.Contains(st.Players[0].Exile, "Counters:P1P1=1") {
		t.Errorf("exile %q dropped Counters, want it kept", st.Players[0].Exile)
	}
	if strings.Contains(st.Players[0].Exile, "Tapped") {
		t.Errorf("exile %q carries Tapped, want it dropped -- Tapped is Battlefield-only", st.Players[0].Exile)
	}
}

// Imprinting round-trips the same way RememberedCards does, and a
// remembered player -- not just a card -- must not show up in
// RememberedCards:, which is a comma id list of cards only.
func TestDumpRememberedAndImprintedCards(t *testing.T) {
	t.Parallel()

	db := testDB(t, "Grizzly Bears", "Llanowar Elves")
	l := load(t, db, "humanbattlefield=Grizzly Bears;Llanowar Elves\n")
	human := l.Game.Players()[0]
	bf := l.Game.Zone(engine.Battlefield, human).Cards()
	bears, elves := bf[0], bf[1]

	l.Game.Card(bears).Memory.Remember(engine.CardEntity(elves))
	l.Game.Card(bears).Memory.Remember(engine.PlayerEntity(human))
	l.Game.Card(bears).Memory.Imprint(elves)

	st := fixture.Dump(l)
	entries := strings.Split(st.Players[0].Battlefield, ";")

	if !strings.Contains(entries[0], "RememberedCards:") {
		t.Fatalf("Grizzly Bears entry %q has no RememberedCards:", entries[0])
	}
	if !strings.Contains(entries[0], "Imprinting:") {
		t.Errorf("Grizzly Bears entry %q has no Imprinting:", entries[0])
	}

	// The remembered id list has to name exactly one card -- Elves -- and the
	// cleanest check is a full reload through Load, which resolves the id
	// back into a CardID rather than string-matching a number that will
	// differ from elves' original handle.
	var buf strings.Builder
	if err := fixture.Write(&buf, st); err != nil {
		t.Fatalf("Write: %v", err)
	}
	st2, err := fixture.Parse(strings.NewReader(buf.String()))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	l2, err := fixture.Load(st2, db, javarand.New(1))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	bf2 := l2.Game.Zone(engine.Battlefield, l2.Game.Players()[0]).Cards()
	remembered := l2.Game.Card(bf2[0]).Memory.Remembered()
	if len(remembered) != 1 {
		t.Fatalf("remembered %v after round trip, want exactly one entry (the player must not survive)", remembered)
	}
	if got, ok := remembered[0].AsCard(); !ok || got != bf2[1] {
		t.Errorf("remembered %v, want Llanowar Elves", remembered)
	}
	if imp := l2.Game.Card(bf2[0]).Memory.Imprinted(); len(imp) != 1 || imp[0] != bf2[1] {
		t.Errorf("imprinted %v, want [Llanowar Elves]", imp)
	}
}

// A player Dump cannot address -- one not named human/ai/p<n> -- is skipped
// rather than guessed at.
func TestDumpSkipsUnaddressablePlayers(t *testing.T) {
	t.Parallel()

	db := testDB(t)
	g := engine.NewGame(db, javarand.New(1), []string{"weird-name"})
	st := fixture.Dump(&fixture.Loaded{Game: g})

	for i, p := range st.Players {
		if p.Named {
			t.Errorf("slot %d is Named, want none -- the only player has an unaddressable name", i)
		}
	}
}

func TestDumpEmptyZoneIsEmptyString(t *testing.T) {
	t.Parallel()

	db := testDB(t)
	l := load(t, db, "humanlife=20\n")
	st := fixture.Dump(l)

	if st.Players[0].Battlefield != "" {
		t.Errorf("battlefield %q, want empty", st.Players[0].Battlefield)
	}
}
