package boosters_test

import (
	"bufio"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/jczastkiewicz/crucible/internal/boosters"
	"github.com/jczastkiewicz/crucible/internal/carddb/compile"
	"github.com/jczastkiewicz/crucible/pkg/javarand"
)

// The oracle's output (crucible/oracle-java/dump-boosters.sh) is the reference
// every layer of the port is tested against (ADR-0049's gate).

type goldenSheet struct {
	key     string
	size    int
	entries []string // "name|edition|collector|art|foil weight"
}

type golden struct {
	templates []string
	picks     map[int]string
	packs     map[int][]string // PACK seed
	tpacks    map[int][]string // TPACK index
	tpackKey  map[int]string
	sheets    []goldenSheet
}

func repoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(file), "..", "..", "..")
}

var (
	goldenOnce sync.Once
	goldenVal  *golden
	goldenErr  error
)

func loadGolden(t *testing.T) *golden {
	t.Helper()
	goldenOnce.Do(func() {
		f, err := os.Open(filepath.Join(repoRoot(t), "crucible", "testdata", "boosters.golden"))
		if err != nil {
			goldenErr = err
			return
		}
		defer func() { _ = f.Close() }()
		g := &golden{picks: map[int]string{}, packs: map[int][]string{}, tpacks: map[int][]string{}, tpackKey: map[int]string{}}
		sc := bufio.NewScanner(f)
		sc.Buffer(make([]byte, 1<<20), 1<<20)
		for sc.Scan() {
			parts := strings.Split(sc.Text(), "\t")
			switch parts[0] {
			case "T":
				g.templates = append(g.templates, parts[1])
			case "PICK":
				n, _ := strconv.Atoi(parts[1])
				g.picks[n] = parts[2]
			case "PACK":
				n, _ := strconv.Atoi(parts[1])
				curPack = func(c string) { g.packs[n] = append(g.packs[n], c) }
			case "TPACK":
				n, _ := strconv.Atoi(parts[1])
				g.tpackKey[n] = parts[2]
				curPack = func(c string) { g.tpacks[n] = append(g.tpacks[n], c) }
			case "C":
				curPack(parts[1])
			case "SHEET":
				size := -1
				if parts[2] != "null" {
					size, _ = strconv.Atoi(parts[2])
				}
				g.sheets = append(g.sheets, goldenSheet{key: parts[1], size: size})
				curSheet = &g.sheets[len(g.sheets)-1]
			case "S":
				curSheet.entries = append(curSheet.entries, parts[1]+" "+parts[2])
			}
		}
		goldenErr = sc.Err()
		goldenVal = g
	})
	if goldenErr != nil {
		t.Fatal(goldenErr)
	}
	return goldenVal
}

var (
	curPack  func(string)
	curSheet *goldenSheet
)

var (
	tableOnce sync.Once
	tableVal  *boosters.Table
	tableErr  error
)

func loadTable(t *testing.T) *boosters.Table {
	t.Helper()
	tableOnce.Do(func() {
		root := repoRoot(t)
		db, err := compile.LoadDB(filepath.Join(root, "forge-gui", "res", "cardsfolder"),
			filepath.Join(root, "forge-gui", "res", "lists", "TypeLists.txt"))
		if err != nil {
			tableErr = err
			return
		}
		tableVal, tableErr = boosters.Load(filepath.Join(root, "forge-gui", "res", "editions"),
			filepath.Join(root, "forge-gui", "res", "blockdata"), db)
	})
	if tableErr != nil {
		t.Fatal(tableErr)
	}
	return tableVal
}

func TestTemplatesIterateInTheOraclesOrder(t *testing.T) {
	g := loadGolden(t)
	tab := loadTable(t)
	var got []string
	for _, tpl := range tab.Templates {
		got = append(got, tpl.Edition.Code)
	}
	if len(got) != len(g.templates) {
		t.Fatalf("%d templates, oracle %d", len(got), len(g.templates))
	}
	for i := range got {
		if got[i] != g.templates[i] {
			t.Fatalf("template %d: %s, oracle %s", i, got[i], g.templates[i])
		}
	}
}

func TestSheetsMatchTheOracle(t *testing.T) {
	g := loadGolden(t)
	tab := loadTable(t)
	gen := boosters.NewGenerator(tab)
	bad := 0
	for _, want := range g.sheets {
		sh, err := gen.Sheet(want.key)
		if err != nil {
			t.Errorf("sheet %q: %v", want.key, err)
			if bad++; bad > 15 {
				t.Fatal("too many sheet errors")
			}
			continue
		}
		got := sh.Dump()
		mismatch := false
		if len(got) != len(want.entries) {
			t.Errorf("sheet %q: %d entries, oracle %d", want.key, len(got), len(want.entries))
			mismatch = true
		} else {
			for i := range got {
				if got[i] != want.entries[i] {
					t.Errorf("sheet %q entry %d: %s, oracle %s", want.key, i, got[i], want.entries[i])
					mismatch = true
					break
				}
			}
		}
		if mismatch {
			if bad++; bad > 25 {
				t.Fatal("too many sheet mismatches")
			}
		}
	}
}

func packLines(cards []*boosters.Print) []string {
	out := make([]string, len(cards))
	for i, c := range cards {
		out[i] = c.Describe()
	}
	return out
}

func samePack(t *testing.T, what string, got, want []string) bool {
	t.Helper()
	if len(got) != len(want) {
		t.Errorf("%s: %d cards, oracle %d\n got %v\nwant %v", what, len(got), len(want), got, want)
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			t.Errorf("%s: card %d is %s, oracle %s", what, i, got[i], want[i])
			return false
		}
	}
	return true
}

// A pack of every template under seed = its index, each generator sharing the
// sheets the earlier ones made, as the oracle does.
func TestPacksOfEveryTemplateMatchTheOracle(t *testing.T) {
	g := loadGolden(t)
	tab := loadTable(t)
	gen := boosters.NewGenerator(tab)
	bad := 0
	for i, tpl := range tab.Templates {
		cards, err := gen.Open(tpl, javarand.New(int64(i)))
		if err != nil {
			t.Errorf("template %s: %v", tpl.Key, err)
			bad++
		} else if !samePack(t, "template "+tpl.Key, packLines(cards), g.tpacks[i]) {
			bad++
		}
		if bad > 12 {
			t.Fatal("too many pack mismatches")
		}
	}
}

// Booster Tutor's own path: the template drawn from all of them, then its pack.
func TestRandomPacksMatchTheOracle(t *testing.T) {
	g := loadGolden(t)
	tab := loadTable(t)
	gen := boosters.NewGenerator(tab)
	for seed := 1; seed <= 12; seed++ {
		tpl, cards, err := gen.OpenRandom(javarand.New(int64(seed)))
		if err != nil {
			t.Fatalf("seed %d: %v", seed, err)
		}
		if tpl.Edition.Code != g.picks[seed] {
			t.Fatalf("seed %d: template %s, oracle %s", seed, tpl.Edition.Code, g.picks[seed])
		}
		samePack(t, "seed "+strconv.Itoa(seed), packLines(cards), g.packs[seed])
	}
}
