// Package boosters makes booster packs the way Forge does (ADR-0049): the
// edition files of forge-gui/res/editions, the card pool, the print sheets and
// the generator, with the iteration orders of the Java collections they use.
//
// The edition files are read as Forge's CardEdition.Reader does, for what
// booster generation needs: the metadata keys, the card sections, the
// BoosterSlots sections and the custom print-sheet sections.
package boosters

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf16"
)

// Rarity is CardRarity.
type Rarity uint8

// The rarities, in CardRarity order.
const (
	BasicLand Rarity = iota
	Common
	Uncommon
	Rare
	MythicRare
	Special
	Token
	Unknown
)

// rarityOf is CardRarity.smartValueOf: the short letter, the enum name or the
// long name, any case, else Unknown.
func rarityOf(s string) Rarity {
	for r, names := range [...][3]string{
		BasicLand: {"BasicLand", "L", "Basic Land"}, Common: {"Common", "C", "Common"},
		Uncommon: {"Uncommon", "U", "Uncommon"}, Rare: {"Rare", "R", "Rare"},
		MythicRare: {"MythicRare", "M", "Mythic Rare"}, Special: {"Special", "S", "Special"},
		Token: {"Token", "T", "Token"}, Unknown: {"Unknown", "?", "Unknown"},
	} {
		for _, n := range names {
			if strings.EqualFold(n, s) {
				return Rarity(r)
			}
		}
	}
	return Unknown
}

// noCollectorNumber is IPaperCard.NO_COLLECTOR_NUMBER.
const noCollectorNumber = "N.A."

// entry is CardEdition.EditionEntry.
type entry struct {
	name      string
	collector string
	rarity    Rarity
	// variant is the "variant" extra parameter of the line, "" when it has none.
	variant string
}

// section is one named group of lines of an edition file.
type section struct {
	name  string
	lines []string
}

// slotSpec is a SealedTemplate slot: Count cards of Spec.
type slotSpec struct {
	spec  string
	count int
}

// boosterSlot is BoosterSlot: the base sheet of a named slot and the sheets
// that replace it with a probability each, in file order (the cumulative
// ranges are built in ascending probability order by a TreeMap).
type boosterSlot struct {
	name string
	base string
	// replaces is the TreeMap<Float, String> of cumulative ranges, ascending;
	// a later line with an equal cumulative value takes the entry over.
	replaces []slotReplace
}

type slotReplace struct {
	upTo  float32
	sheet string
}

// Edition is the part of a CardEdition the booster code reads.
type Edition struct {
	Name, Code, Code2 string
	Alias             string
	Date              time.Time
	Covers            int
	// Templates by type ("Draft", "Collector", "Set").
	templates   map[string][]slotSpec
	slotted     bool
	slots       []boosterSlot
	slotNames   []string
	foilChance  float64
	foilAlways  bool
	foilType    string
	foilSheet   string
	replaceProb float64
	replaceSlot string
	mustContain string
	replaceFrom string
	sheetRepl1  string
	sheetRepl2  string

	// cards is the card sections in the iteration order of Java's
	// ArrayListMultimap (a HashMap of the section names); custom is the other
	// sections, by name.
	cards        []entry
	custom       []section
	cardSections map[string][]entry
	// sectionOrder is the card sections in cardsInSet's order.
	sectionOrder []string
	// traditional is the lower-cased collector numbers of the "cards" section.
	traditional map[string]bool
}

var cardPattern = regexp.MustCompile(`^(?:(.?[0-9A-Z-]+\S*[A-Z]*)\s)?(?:([SCURML])\s)?([^@$]+)(?: @([^$]*))?(?: \$\{(.+)\})?$`)

// cardSections are EditionSectionWithCollectorNumbers.
var cardSections = map[string]bool{
	"cards": true, "special slot": true, "precon product": true, "borderless": true, "etched": true,
	"showcase": true, "full art": true, "extended art": true, "alternate art": true, "retro frame": true,
	"buy a box": true, "promo": true, "prerelease promo": true, "bundle": true, "box topper": true,
	"jumpstart": true, "rebalanced": true, "eternal": true, "conjured": true, "scheme": true, "printsheets": true,
}

// parseSections is FileSection.parseSections: [name] headers, # comments and
// blank lines dropped, the sections in file order.
func parseSections(text string) []section {
	var out []section
	idx := map[string]int{}
	cur := -1
	for _, raw := range strings.Split(text, "\n") {
		line := strings.TrimSpace(raw)
		if strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			name := line[1 : len(line)-1]
			i, ok := idx[name]
			if !ok {
				i = len(out)
				idx[name] = i
				out = append(out, section{name: name})
			}
			cur = i
		} else if cur >= 0 && line != "" {
			out[cur].lines = append(out[cur].lines, line)
		}
	}
	return out
}

// metadata is FileSection.parse with "=": key to value, keys folded.
func metadata(lines []string) map[string]string {
	m := map[string]string{}
	for _, l := range lines {
		k, v, ok := strings.Cut(l, "=")
		if !ok {
			continue
		}
		m[strings.ToLower(strings.TrimSpace(k))] = strings.TrimSpace(v)
	}
	return m
}

func (m metaMap) get(key, def string) string {
	if v, ok := m[strings.ToLower(key)]; ok {
		return v
	}
	return def
}

type metaMap map[string]string

func parseDate(s string) time.Time {
	if len(s) <= 7 {
		s += "-01"
	}
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		return time.Now()
	}
	return t
}

// parseSlots is SealedTemplate.Reader.parseSlots: top-level commas separate
// "<count> <spec>" items, a ; in a spec is a comma.
func parseSlots(s string) ([]slotSpec, error) {
	var out []slotSpec
	for _, item := range splitTop(s, ',') {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		cnt, spec, _ := strings.Cut(item, " ")
		n, err := strconv.Atoi(cnt)
		if err != nil {
			return nil, fmt.Errorf("boosters: slot %q: %w", item, err)
		}
		out = append(out, slotSpec{spec: strings.ReplaceAll(strings.TrimSpace(spec), ";", ","), count: n})
	}
	return out, nil
}

// splitTop splits s on sep outside parentheses (TextUtil.splitWithParenthesis
// with ( and )).
func splitTop(s string, sep byte) []string {
	var out []string
	depth, start := 0, 0
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '(':
			depth++
		case ')':
			depth--
		case sep:
			if depth == 0 {
				out = append(out, s[start:i])
				start = i + 1
			}
		}
	}
	return append(out, s[start:])
}

// parseEdition reads one edition file.
func parseEdition(text string) (*Edition, error) {
	secs := parseSections(text)
	var metaLines []string
	for _, s := range secs {
		if s.name == "metadata" {
			metaLines = s.lines
		}
	}
	meta := metaMap(metadata(metaLines))
	e := &Edition{
		Name: meta.get("name", ""), Code: meta.get("code", ""), Date: parseDate(meta.get("date", "")),
		templates: map[string][]slotSpec{},
	}
	e.Code2 = meta.get("code2", e.Code)
	e.Alias = meta.get("alias", "")
	if n, err := strconv.Atoi(meta.get("BoosterCovers", "1")); err == nil {
		e.Covers = n
	} else {
		e.Covers = 1
	}

	slotNames := []string(nil)
	if v, ok := meta["boosterslots"]; ok {
		e.slotted = true
		slotNames = strings.Split(v, ",")
		e.slotNames = slotNames
	}
	inSlots := map[string]bool{}
	for _, n := range slotNames {
		inSlots[n] = true
	}

	var cardSecNames []string
	byName := map[string][]entry{}
	for _, s := range secs {
		switch {
		case s.name == "metadata" || s.name == "tokens" || s.name == "other":
		case strings.HasSuffix(s.name, "Types"):
		case cardSections[s.name]:
			cardSecNames = append(cardSecNames, s.name)
			for _, line := range s.lines {
				m := cardPattern.FindStringSubmatch(line)
				if m == nil {
					continue
				}
				byName[s.name] = append(byName[s.name], entry{name: m[3], collector: m[1], rarity: rarityOf(m[2]), variant: extraParam(m[5], "variant")})
			}
		case inSlots[s.name]:
			bs, err := parseBoosterSlot(s.name, s.lines)
			if err != nil {
				return nil, err
			}
			e.slots = append(e.slots, bs)
		default:
			e.custom = append(e.custom, s)
		}
	}
	// Java's ArrayListMultimap iterates its sections in the order of a HashMap
	// of the section names.
	for _, i := range hashMapOrder(cardSecNames) {
		name := cardSecNames[i]
		e.cards = append(e.cards, byName[name]...)
		e.sectionOrder = append(e.sectionOrder, name)
	}
	// The constructor then sorts cardsInSet (EditionEntry.compareTo); the
	// section order only breaks ties.
	sort.SliceStable(e.cards, func(i, j int) bool { return entryLess(e.cards[i], e.cards[j]) })
	e.cardSections = byName
	e.traditional = map[string]bool{}
	for _, en := range byName["cards"] {
		if en.collector != "" {
			e.traditional[strings.ToLower(en.collector)] = true
		}
	}

	if b, ok := meta["booster"]; ok {
		t, err := parseSlots(b)
		if err != nil {
			return nil, err
		}
		e.templates["Draft"] = t
	}
	for _, typ := range []string{"Draft", "Collector", "Set"} {
		if v, ok := meta[strings.ToLower(typ+"Booster")]; ok {
			t, err := parseSlots(v)
			if err != nil {
				return nil, err
			}
			e.templates[typ] = t
			if typ == "Draft" {
				e.slotted = false
			}
		}
	}
	// The slot sections apply only to the "Booster=" template (CardEdition.java:811-822).
	if _, ok := meta["booster"]; !ok {
		e.slotted = false
	}
	switch strings.ToLower(meta.get("foil", "newstyle")) {
	case "oldstyle", "classic":
		e.foilType = "OLD_STYLE"
	case "newstyle", "modern":
		e.foilType = "MODERN"
	default:
		e.foilType = "NOT_SUPPORTED"
	}
	rc := strings.SplitN(meta.get("ChanceReplaceCommonWith", "0F Common"), " ", 2)
	p, err := strconv.ParseFloat(strings.TrimSuffix(rc[0], "F"), 64)
	if err != nil {
		return nil, fmt.Errorf("boosters: %s: ChanceReplaceCommonWith: %w", e.Code, err)
	}
	e.replaceProb, e.replaceSlot = p, rc[len(rc)-1]
	e.foilChance = float64(float32(21.43)) / 100.0
	if v, ok := meta["foilchanceinbooster"]; ok {
		f, err := strconv.ParseFloat(v, 64)
		if err != nil {
			return nil, fmt.Errorf("boosters: %s: FoilChanceInBooster: %w", e.Code, err)
		}
		e.foilChance = f / 100.0
	}
	e.foilAlways = !strings.EqualFold(meta.get("FoilAlwaysInCommonSlot", "true"), "false")
	e.foilSheet = meta.get("AdditionalSheetForFoils", "")
	e.mustContain = meta.get("BoosterMustContain", "")
	e.replaceFrom = meta.get("BoosterReplaceSlotFromPrintSheet", "")
	e.sheetRepl1 = meta.get("SheetReplaceCardFromSheet", "")
	e.sheetRepl2 = meta.get("SheetReplaceCardFromSheet2", "")
	return e, nil
}

// parseBoosterSlot is BoosterSlot.parseSlot: Base=<spec> and Replace=<p>F <spec>.
func parseBoosterSlot(name string, lines []string) (boosterSlot, error) {
	bs := boosterSlot{name: name}
	var start float32
	for _, l := range lines {
		if strings.HasPrefix(l, "#") {
			continue
		}
		k, v, ok := strings.Cut(l, "=")
		if !ok {
			continue
		}
		switch strings.TrimSpace(k) {
		case "Base":
			bs.base = strings.TrimSpace(v)
		case "Replace":
			c, sheet, _ := strings.Cut(strings.TrimSpace(v), " ")
			f, err := strconv.ParseFloat(strings.TrimSuffix(c, "F"), 32)
			if err != nil {
				return bs, fmt.Errorf("boosters: slot %s: Replace %q: %w", name, v, err)
			}
			start += float32(f)
			bs.put(start, strings.TrimSpace(sheet))
		}
	}
	return bs, nil
}

// loadEditions reads every *.txt of dir, ordered as StaticData's edition
// collection is: a case-insensitive TreeMap by code.
func loadEditions(dir string) ([]*Edition, error) {
	files, err := filepath.Glob(filepath.Join(dir, "*.txt"))
	if err != nil {
		return nil, err
	}
	var out []*Edition
	for _, f := range files {
		raw, err := os.ReadFile(f)
		if err != nil {
			return nil, err
		}
		e, err := parseEdition(string(raw))
		if err != nil {
			return nil, fmt.Errorf("%s: %w", f, err)
		}
		out = append(out, e)
	}
	sort.SliceStable(out, func(i, j int) bool { return strings.ToLower(out[i].Code) < strings.ToLower(out[j].Code) })
	return out, nil
}

// entryLess is EditionEntry.compareTo < 0: the name without case, then the
// collector number as a sortable string, then the rarity.
func entryLess(a, b entry) bool {
	if c := compareIgnoreCase(a.name, b.name); c != 0 {
		return c < 0
	}
	if x, y := sortableCollector(a.collector), sortableCollector(b.collector); x != y {
		return x < y
	}
	return a.rarity < b.rarity
}

// compareIgnoreCase is String.compareToIgnoreCase: per UTF-16 unit, equal when
// the upper-cased or the lower-cased units are.
func compareIgnoreCase(a, b string) int {
	ua, ub := utf16Units(a), utf16Units(b)
	n := min(len(ua), len(ub))
	for i := 0; i < n; i++ {
		x, y := ua[i], ub[i]
		if x == y {
			continue
		}
		x, y = unitUpper(x), unitUpper(y)
		if x != y {
			x, y = unitLower(x), unitLower(y)
			if x != y {
				return int(x) - int(y)
			}
		}
	}
	return len(ua) - len(ub)
}

func utf16Units(s string) []uint16 { return utf16.Encode([]rune(s)) }

func unitUpper(u uint16) uint16 { return uint16(unicode.ToUpper(rune(u))) }
func unitLower(u uint16) uint16 { return uint16(unicode.ToLower(rune(u))) }

// sortableCollector is CardEdition.getSortableCollectorNumber: a number
// zero-padded to five digits, with any other characters attached.
func sortableCollector(cn string) string {
	in := cn
	if in == "" {
		in = "50000"
	}
	if n, err := strconv.ParseInt(strings.TrimPrefix(in, "+"), 10, 32); err == nil && !strings.HasPrefix(in, "+-") {
		return fmt.Sprintf("%05d", n)
	}
	nonNum := strings.Map(func(r rune) rune {
		if r >= '0' && r <= '9' {
			return -1
		}
		return r
	}, in)
	onlyNum := strings.Map(func(r rune) rune {
		if r >= '0' && r <= '9' {
			return r
		}
		return -1
	}, in)
	n, err := strconv.ParseInt(onlyNum, 10, 32)
	if err != nil {
		n = 0
	}
	if n > 0 && strings.HasPrefix(in, onlyNum) {
		return fmt.Sprintf("%05d", n) + nonNum
	}
	return nonNum + fmt.Sprintf("%05d", n)
}

var extraParamPattern = regexp.MustCompile(`"([^"]+)"\s*:\s*"([^"]+)",?`)

// extraParam is the value of key in a line's ${"key": "value", ...} text, keys
// compared as CardEdition.Reader stores them (trimmed, lower case).
func extraParam(text, key string) string {
	if strings.TrimSpace(text) == "" {
		return ""
	}
	loc := extraParamPattern.FindStringIndex(text)
	if loc == nil || loc[0] != 0 {
		return ""
	}
	for _, m := range extraParamPattern.FindAllStringSubmatch(text, -1) {
		if strings.ToLower(strings.TrimSpace(m[1])) == key {
			return strings.TrimSpace(m[2])
		}
	}
	return ""
}

// put is TreeMap.put on the ranges: ascending order, an equal key replaced.
func (b *boosterSlot) put(upTo float32, sheet string) {
	i := sort.Search(len(b.replaces), func(i int) bool { return b.replaces[i].upTo >= upTo })
	if i < len(b.replaces) && b.replaces[i].upTo == upTo {
		b.replaces[i].sheet = sheet
		return
	}
	b.replaces = append(b.replaces, slotReplace{})
	copy(b.replaces[i+1:], b.replaces[i:])
	b.replaces[i] = slotReplace{upTo: upTo, sheet: sheet}
}
