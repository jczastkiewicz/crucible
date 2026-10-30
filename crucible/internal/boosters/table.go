// The immutable booster data of a process: the editions, the card pool, the
// print sheets Forge reads from the edition files and printsheets.txt, and the
// booster templates in the order StaticData.getBoosters iterates them
// (ADR-0049).

package boosters

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/jczastkiewicz/crucible/internal/carddb/compile"
)

// Template is a booster SealedTemplate with its owner.
type Template struct {
	// Key is the name it is stored under: the edition code, with the type
	// appended unless it is "Draft".
	Key     string
	Edition *Edition
	Type    string
	Slots   []slotSpec
	// withSlots is a SealedTemplateWithSlots: the edition's BoosterSlots
	// sections drive each slot.
	withSlots bool
}

// Table is the immutable data.
type Table struct {
	Pool      *Pool
	variants  *Pool
	Templates []*Template
	static    map[string]staticSheet
}

// staticSheet is a sheet read from the data files, or the reason it could not
// be built here (a bin Java would treeify): that only matters if a pack
// reaches it.
type staticSheet struct {
	sheet *Sheet
	err   error
}

// Load reads the editions of editionsDir and the print sheets of
// blockDataDir/printsheets.txt over the cards of db.
func Load(editionsDir, blockDataDir string, db *compile.DB) (*Table, error) {
	if db == nil {
		return nil, fmt.Errorf("boosters: no card database")
	}
	eds, err := loadEditions(editionsDir)
	if err != nil {
		return nil, err
	}
	pool, err := buildPool(eds, db, false)
	if err != nil {
		return nil, err
	}
	variants, err := buildPool(eds, db, true)
	if err != nil {
		return nil, err
	}
	t := &Table{Pool: pool, variants: variants, static: map[string]staticSheet{}}

	// CardEdition.getBoosterGenerator: a TreeMap, keys without case.
	for _, e := range eds {
		for _, typ := range []string{"Draft", "Collector", "Set"} {
			slots, ok := e.templates[typ]
			if !ok {
				continue
			}
			key := e.Code
			if typ != "Draft" {
				key += typ
			}
			t.Templates = append(t.Templates, &Template{Key: key, Edition: e, Type: typ, Slots: slots,
				withSlots: typ == "Draft" && e.slotted})
		}
	}
	sort.SliceStable(t.Templates, func(i, j int) bool { return fold(t.Templates[i].Key) < fold(t.Templates[j].Key) })

	// printsheets.txt first, then the edition sheets over it
	// (PrintSheet.initializePrintSheets).
	if blockDataDir != "" {
		raw, err := os.ReadFile(filepath.Join(blockDataDir, "printsheets.txt"))
		if err != nil {
			return nil, err
		}
		for _, s := range parseSections(string(raw)) {
			sh, err := t.cardPoolSheet(s.name, s.lines)
			t.static[fold(s.name)] = staticSheet{sheet: sh, err: err}
		}
	}
	for _, e := range eds {
		for _, name := range e.sectionOrder {
			if name == "conjured" {
				continue
			}
			sh := newSheet(e.Code + " " + name)
			var serr error
			for _, en := range e.cardSections[name] {
				pr, err := pool.editionCard(en, e)
				if err == nil {
					err = sh.add(pr, 1)
				}
				if err != nil {
					serr = err
					break
				}
			}
			t.static[fold(sh.name)] = staticSheet{sheet: sh, err: serr}
		}
		for _, c := range e.custom {
			sh, err := t.cardPoolSheet(e.Code+" "+c.name, c.lines)
			t.static[fold(e.Code+" "+c.name)] = staticSheet{sheet: sh, err: err}
		}
	}
	return t, nil
}

// editionCard is getPrintSheetsBySection's cardDb.getCard(name, code,
// collectorNumber). A card the database does not know is nil, which a sheet
// ignores.
func (p *Pool) editionCard(en entry, e *Edition) (*Print, error) {
	if len(p.byName[fold(en.name)]) == 0 {
		return nil, nil
	}
	req := en.name + "|" + e.Code
	if en.collector != "" {
		req += "|[" + en.collector + "]"
	}
	return p.getCard(req)
}

var cardLine = regexp.MustCompile(`^(?:(\d+)\s+)?(.*?)$`)

// cardPoolSheet is a PrintSheet over CardPool.fromCardList(lines): each line
// "<n> <name>|<set>[|<art>]" adds the print it names n times.
func (t *Table) cardPoolSheet(name string, lines []string) (*Sheet, error) {
	sh := newSheet(name)
	for _, line := range lines {
		if strings.HasPrefix(line, ";") || strings.HasPrefix(line, "#") {
			continue
		}
		m := cardLine.FindStringSubmatch(strings.TrimSpace(line))
		if m == nil || strings.TrimSpace(m[2]) == "" {
			continue
		}
		n := 1
		if m[1] != "" {
			n, _ = strconv.Atoi(m[1])
		}
		req := m[2]
		// A name ending in + asks for the foil (CardDb.CardRequest.isFoilCardName).
		foil := false
		nameEnd := strings.Index(req, "|")
		if nameEnd < 0 {
			nameEnd = len(req)
		}
		if strings.HasSuffix(strings.TrimSpace(req[:nameEnd]), "+") {
			foil = true
			req = strings.TrimSuffix(strings.TrimSpace(req[:nameEnd]), "+") + req[nameEnd:]
		}
		// CardPool.add asks the common database first, then the variant one.
		pr, err := t.Pool.getCard(req)
		if err == nil && pr == nil {
			pr, err = t.variants.getCard(req)
		}
		if err != nil {
			return nil, fmt.Errorf("print sheet %q line %q: %w", name, line, err)
		}
		if pr == nil {
			// CardPool.add: "An unsupported card was requested": a placeholder
			// of the name, in the requested edition with the rarity that
			// edition lists the name under, else Unknown.
			pr = t.Pool.unsupported(req)
		}
		if foil {
			pr = pr.Foiled()
		}
		if err := sh.add(pr, n); err != nil {
			return nil, err
		}
	}
	return sh, nil
}

// staticSheet is BoosterGenerator.tryGetStaticSheet: the sheet named, its
// first word taken as an edition code or alias and replaced by the edition's
// code. nil when there is none.
func (t *Table) staticSheet(name string) (*Sheet, error) {
	first, _, _ := strings.Cut(name, " ")
	e := t.Pool.edition(first)
	if e == nil {
		return nil, fmt.Errorf("boosters: sheet %q names no edition", name)
	}
	if e.Code != first {
		name = e.Code + strings.TrimPrefix(name, first)
	}
	st, ok := t.static[fold(name)]
	if !ok {
		return nil, nil
	}
	return st.sheet, st.err
}

// byCode is StaticData.getEditions().contains(code): an edition of this code,
// not alias; false for "".
func (t *Table) byCode(code string) (*Edition, bool) {
	if code == "" {
		return nil, false
	}
	e, ok := t.Pool.byCode[fold(code)]
	return e, ok
}
