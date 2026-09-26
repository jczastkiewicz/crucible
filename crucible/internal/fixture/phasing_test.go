package fixture_test

import (
	"strings"
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
	"github.com/jczastkiewicz/crucible/internal/fixture"
	"github.com/jczastkiewicz/crucible/pkg/javarand"
)

// A phased-out permanent round-trips in place through Dump, Write, Parse
// and Load under Java's own PhasedOut:P<seat> spelling (GameState.java:
// 309-311, :1302-1304), and Dump writes it even though the battlefield
// enumeration hides it (ADR-0021's fixture opt-in).
func TestPhasedOutRoundTripsInPlace(t *testing.T) {
	t.Parallel()

	db := testDB(t, "Grizzly Bears", "Llanowar Elves", "Mountain")
	l := load(t, db, "humanlife=20\nailife=20\n"+
		"humanbattlefield=Grizzly Bears;Llanowar Elves|PhasedOut:AI;Mountain\n")
	g := l.Game
	human := g.Players()[0]
	if got := len(g.Zone(engine.Battlefield, human).Cards()); got != 2 {
		t.Fatalf("visible battlefield = %d cards, want 2", got)
	}

	st := fixture.Dump(l)
	if !strings.Contains(st.Players[0].Battlefield, "Llanowar Elves|Id:2|PhasedOut:P1;Mountain") {
		t.Errorf("battlefield %q does not keep the phased-out elf in place as PhasedOut:P1", st.Players[0].Battlefield)
	}

	var buf strings.Builder
	if err := fixture.Write(&buf, st); err != nil {
		t.Fatalf("Write: %v", err)
	}
	st2, err := fixture.Parse(strings.NewReader(buf.String()))
	if err != nil {
		t.Fatalf("Parse(Write(x)): %v", err)
	}
	l2, err := fixture.Load(st2, db, javarand.New(1))
	if err != nil {
		t.Fatalf("Load(Parse(Write(x))): %v", err)
	}
	all := l2.Game.Zone(engine.Battlefield, l2.Game.Players()[0]).CardsIncludingPhasedOut()
	if len(all) != 3 {
		t.Fatalf("round trip: %d cards, want 3", len(all))
	}
	elf := l2.Game.Card(all[1])
	if elf.UncopiedDef().Name != "Llanowar Elves" || elf.PhasedOutFor() != l2.Game.Players()[1] {
		t.Errorf("round trip: %q phased out for %v, want Llanowar Elves for the second seat", elf.UncopiedDef().Name, elf.PhasedOutFor())
	}
}

func TestLoadPhasedOutPlayerSpellings(t *testing.T) {
	t.Parallel()

	db := testDB(t, "Grizzly Bears")
	for _, tc := range []struct {
		value string
		seat  int
	}{
		{"HUMAN", 0},
		{"human", 0},
		{"AI", 1},
		{"P0", 0},
		{"P1", 1},
	} {
		l := load(t, db, "humanlife=20\nailife=20\nhumanbattlefield=Grizzly Bears|PhasedOut:"+tc.value+"\n")
		g := l.Game
		id := g.Zone(engine.Battlefield, g.Players()[0]).CardsIncludingPhasedOut()[0]
		if got := g.Card(id).PhasedOutFor(); got != g.Players()[tc.seat] {
			t.Errorf("PhasedOut:%s -> %v, want seat %d", tc.value, got, tc.seat)
		}
	}

	for _, bad := range []string{"P7", "Bob", "PhasedOut"} {
		st, err := fixture.Parse(strings.NewReader("humanlife=20\nailife=20\nhumanbattlefield=Grizzly Bears|PhasedOut:" + bad + "\n"))
		if err != nil {
			t.Fatalf("Parse: %v", err)
		}
		if _, err := fixture.Load(st, db, javarand.New(1)); err == nil {
			t.Errorf("PhasedOut:%s: got nil error", bad)
		}
	}

	l := load(t, db, "humanlife=20\nailife=20\nhumanhand=Grizzly Bears|PhasedOut:P0\n")
	if len(l.Unapplied) != 1 {
		t.Errorf("PhasedOut on a card in hand: unapplied = %v, want one entry", l.Unapplied)
	}
}
