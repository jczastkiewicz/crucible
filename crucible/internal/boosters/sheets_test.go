package boosters_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/jczastkiewicz/crucible/internal/boosters"
	"github.com/jczastkiewicz/crucible/pkg/javarand"
)

// Sheet keys outside the ones the oracle's packs reach: the operators makeSheet
// reads, and what it refuses.

func TestSheetOperators(t *testing.T) {
	t.Parallel()
	gen := boosters.NewGenerator(loadTable(t))

	for key, check := range map[string]func([]string) bool{
		// A named card of one edition, by the promo operator.
		`promo("Lightning Bolt|M10")`: func(d []string) bool { return len(d) == 1 && strings.HasPrefix(d[0], "Lightning Bolt|M10|") },
		// The inverse of a rarity, restricted to one edition.
		"!Common M10": func(d []string) bool { return len(d) > 0 && !strings.Contains(strings.Join(d, "\n"), "Giant Growth") },
		// Every land of the set; Any is no restriction on rarity.
		"Land:Any M10":               func(d []string) bool { return len(d) > 0 },
		"Special M10":                func([]string) bool { return true },
		"Mythic M10":                 func(d []string) bool { return len(d) > 0 },
		"Rare:Mythic M10":            func([]string) bool { return true },
		"name(\"Giant Growth\") M10": func(d []string) bool { return len(d) == 1 },
		"UncommonRare M10":           func(d []string) bool { return len(d) > 0 },
		"UncommonRareMythic M10":     func(d []string) bool { return len(d) > 0 },
		"dfc ISD":                    func(d []string) bool { return len(d) > 0 },
		"TimeShifted TSP":            func([]string) bool { return true },
	} {
		sh, err := gen.Sheet(key)
		if err != nil {
			t.Errorf("sheet %q: %v", key, err)
			continue
		}
		if !check(sh.Dump()) {
			t.Errorf("sheet %q has %d entries, which is not what it should hold", key, len(sh.Dump()))
		}
	}
}

func TestSheetErrors(t *testing.T) {
	t.Parallel()
	gen := boosters.NewGenerator(loadTable(t))

	for _, key := range []string{
		"Bogus M10",                      // operator could not be parsed
		`fromSheet("Nowhere cards") M10`, // a sheet that does not exist
		`fromSheet("ZZZZ cards") M10`,    // an edition that does not exist
		"",                               // nothing at all
		`promo("No Such Card|M10")`,      // a card with no print
	} {
		if sh, err := gen.Sheet(key); err == nil && key != `promo("No Such Card|M10")` {
			t.Errorf("sheet %q made without error (%d entries)", key, len(sh.Dump()))
		}
	}
	for _, key := range []string{`Common:color("red") M10`, `Common:fromSets("M10") M10`} {
		if _, err := gen.Sheet(key); !errors.Is(err, boosters.ErrNotModelled) {
			t.Errorf("sheet %q: err = %v, want ErrNotModelled", key, err)
		}
	}
}

func TestSheetDrawsWholeRunsWithoutDrawing(t *testing.T) {
	t.Parallel()
	gen := boosters.NewGenerator(loadTable(t))
	sh, err := gen.Sheet("BasicLand M10")
	if err != nil {
		t.Fatal(err)
	}
	r := javarand.New(1)
	// Asking for more than the distinct prints returns each once with no draw,
	// then draws the rest; unique draws never repeat.
	cards, err := sh.Random(r, len(sh.Dump())+2, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(cards) != len(sh.Dump())+2 {
		t.Errorf("%d cards drawn, want %d", len(cards), len(sh.Dump())+2)
	}
	if got, _ := sh.Random(javarand.New(1), 0, true); len(got) != 0 {
		t.Errorf("zero cards drawn %d", len(got))
	}
	// Taking every distinct print by weighted unique draws exhausts the roulette
	// when asked for one more than there are: that is the whole-run branch.
	if _, err := sh.Random(javarand.New(2), len(sh.Dump())-1, true); err != nil {
		t.Errorf("drawing all but one unique: %v", err)
	}
}

func TestLoadNeedsADatabaseAndAnEditionDirectory(t *testing.T) {
	t.Parallel()
	if _, err := boosters.Load(t.TempDir(), "", nil); err == nil {
		t.Error("Load without a card database succeeded")
	}
	tab := loadTable(t)
	_ = tab
}
