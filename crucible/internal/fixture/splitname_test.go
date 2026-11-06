package fixture_test

import (
	"strings"
	"testing"

	"github.com/jczastkiewicz/crucible/internal/fixture"
	"github.com/jczastkiewicz/crucible/pkg/javarand"
)

// TestSplitCardLoadsUnderItsPaperName pins that a Split card loads under the
// name GameState writes for it (GameState.java:298, both halves joined with
// " // ") as well as under its first half, and dumps back under the paper
// name, so a Java post-state reads back.
func TestSplitCardLoadsUnderItsPaperName(t *testing.T) {
	t.Parallel()

	for _, name := range []string{"Left Hall", "Left Hall // Right Hall"} {
		l := load(t, roomDB(t), "humanlife=20\nailife=20\nhumangraveyard="+name+"|Id:1\n")
		var buf strings.Builder
		if err := fixture.Write(&buf, fixture.Dump(l)); err != nil {
			t.Fatalf("Write: %v", err)
		}
		if want := "humangraveyard=Left Hall // Right Hall|Id:1"; !strings.Contains(buf.String(), want) {
			t.Errorf("%q dumped as %q, want it to contain %q", name, buf.String(), want)
		}
	}
}

// TestSplitCardNameMustMatchBothHalves pins the loader's error on a joined
// name whose second half is not the card's.
func TestSplitCardNameMustMatchBothHalves(t *testing.T) {
	t.Parallel()

	st, err := fixture.Parse(strings.NewReader("humangraveyard=Left Hall // Wrong Hall\n"))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if _, err := fixture.Load(st, roomDB(t), javarand.New(1)); err == nil {
		t.Error("a mismatched split name loaded, want an error")
	}
}
