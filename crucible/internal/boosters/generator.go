// BoosterGenerator and the pack path of MakeCardEffect (ADR-0049), drawing from
// a *javarand.Rand in the order Java does: the oracle (boosters.golden) is the
// test of that order.

package boosters

import (
	"errors"
	"fmt"
	"math"
	"strings"

	"github.com/jczastkiewicz/crucible/pkg/javarand"
)

// ErrNotModelled marks what Java does and this port does not: a bin it would
// treeify, a sheet operator not ported. A pack that meets it is not made.
var ErrNotModelled = errors.New("boosters: not modelled")

// Generator builds packs from a Table. It holds the sheets already made, which
// Java keeps in a static map: one Generator per Game.
type Generator struct {
	t     *Table
	cache map[string]*Sheet
}

// NewGenerator is a Generator over t.
func NewGenerator(t *Table) *Generator {
	return &Generator{t: t, cache: map[string]*Sheet{}}
}

// Sheet is the sheet for a key, made on first use.
func (g *Generator) Sheet(key string) (*Sheet, error) { return g.sheet(key) }

// Dump is the sheet's entries in iteration order as the oracle writes them:
// "name|edition|collector|art|foil weight".
func (s *Sheet) Dump() []string {
	var out []string
	for _, i := range s.entries() {
		p := s.prints[i]
		out = append(out, fmt.Sprintf("%s|%s|%s|%d|%t %d", p.Name, p.Edition, p.Collector, p.Art, p.Foil, s.weights[i]))
	}
	return out
}

// Describe is the print as the oracle writes a card: "name|edition|collector|art|foil".
func (p *Print) Describe() string {
	return fmt.Sprintf("%s|%s|%s|%d|%t", p.Name, p.Edition, p.Collector, p.Art, p.Foil)
}

// PickTemplate is Aggregates.random over the booster templates (a Map's
// values, not a List): one draw of the full 32-bit range per template, the
// lowest wins, the first on a tie.
func (g *Generator) PickTemplate(r *javarand.Rand) *Template {
	var pick *Template
	lowest := int32(math.MaxInt32)
	for _, t := range g.t.Templates {
		if n := r.Int32(); n < lowest {
			lowest, pick = n, t
		}
	}
	return pick
}

// OpenRandom is what MakeCardEffect does for Booster$: a template chosen by
// PickTemplate, then its pack.
func (g *Generator) OpenRandom(r *javarand.Rand) (*Template, []*Print, error) {
	t := g.PickTemplate(r)
	if t == nil {
		return nil, nil, fmt.Errorf("boosters: no booster templates")
	}
	cards, err := g.Open(t, r)
	return t, cards, err
}

// specialSets are BoosterPack.specialSets: their packs have one cover.
var specialSets = map[string]bool{"Black": true, "Blue": true, "Green": true, "Red": true, "White": true, "Colorless": true}

// Open is new BoosterPack(edition, template).getCards(): the cover draw the
// constructor makes and never uses (BoosterPack.java:57-61), then the pack.
func (g *Generator) Open(t *Template, r *javarand.Rand) ([]*Print, error) {
	if !specialSets[t.Edition.Code] && t.Edition.Code != "?" {
		covers := t.Edition.Covers
		if covers <= 0 {
			return nil, fmt.Errorf("boosters: %s: BoosterCovers %d", t.Edition.Code, covers)
		}
		r.Int32n(int32(covers))
	}
	if t.withSlots {
		return g.slotted(t, r)
	}
	return g.legacy(t, r)
}

// hasSlot is SealedTemplate.hasSlot: a slot whose type, before any space, ! or :, is s.
func (t *Template) hasSlot(s string) bool {
	for _, sl := range t.Slots {
		if slotType(sl.spec) == s {
			return true
		}
	}
	return false
}

// slotType is spec.split("[ :!]")[0].
func slotType(spec string) string {
	if i := strings.IndexAny(spec, " :!"); i >= 0 {
		return spec[:i]
	}
	return spec
}

// slotted is BoosterGenerator.getBoosterPack(SealedTemplateWithSlots): each
// slot's cards are drawn one by one from the replacement ranges of its
// BoosterSlot, grouped by the sheet they name, and each group drawn from its sheet.
func (g *Generator) slotted(t *Template, r *javarand.Rand) ([]*Print, error) {
	named := map[string]*boosterSlot{}
	for i := range t.Edition.slots {
		named[t.Edition.slots[i].name] = &t.Edition.slots[i]
	}
	var result []*Print
	for _, slot := range t.Slots {
		spec := strings.TrimSpace(slot.spec)
		convertAll := strings.HasSuffix(spec, "+")
		spec = strings.TrimSuffix(spec, "+")
		bs := named[spec]
		if bs == nil {
			return nil, fmt.Errorf("boosters: %s: no BoosterSlot %q", t.Edition.Code, spec)
		}
		// Stream.generate(replaceSlot).limit(n) grouped into a HashMap.
		var keys []string
		counts := map[string]int{}
		for range slot.count {
			sheet, ok := bs.replaceSlot(r)
			if !ok {
				continue
			}
			if _, seen := counts[sheet]; !seen {
				keys = append(keys, sheet)
			}
			counts[sheet]++
		}
		for _, i := range hashMapOrderFrom(keys, 16) {
			name, n := keys[i], counts[keys[i]]
			if name == "" || n == 0 {
				continue
			}
			convertThis := strings.HasSuffix(name, "+")
			name = strings.TrimSuffix(name, "+")
			ps, err := g.sheet(name + " " + t.Edition.Code)
			if err != nil {
				if errors.Is(err, ErrNotModelled) {
					return nil, err
				}
				if ps, err = g.sheet(name); err != nil {
					return nil, err
				}
			}
			drawn, err := ps.random(r, n, true)
			if err != nil {
				return nil, err
			}
			for _, p := range drawn {
				if convertAll || convertThis {
					p = p.Foiled()
				}
				result = append(result, p)
			}
		}
	}
	return result, nil
}

// replaceSlot is BoosterSlot.replaceSlot: one float draw; the sheet of the
// first range it falls under, else the base. ok is false for a base of null.
func (b *boosterSlot) replaceSlot(r *javarand.Rand) (string, bool) {
	rnd := r.Float32()
	for _, rep := range b.replaces {
		if rnd < rep.upTo {
			return rep.sheet, true
		}
	}
	return b.base, b.base != ""
}

// legacy is BoosterGenerator.getBoosterPackInternal for a template without
// BoosterSlots sections.
func (g *Generator) legacy(t *Template, r *javarand.Rand) ([]*Print, error) {
	ed := t.Edition
	var result []*Print
	nSlots := len(t.Slots)

	hasFoil := nSlots > 0 && r.Float64() < ed.foilChance && ed.foilType != "NOT_SUPPORTED"
	foilAtEnd := hasFoil && ed.foilAlways

	foilCard := Unknown
	for foilCard == Unknown {
		switch n := r.Int32n(10) + 1; n {
		case 1:
			foilCard = Rare
		case 2, 3:
			foilCard = Uncommon
		case 4, 5, 6:
			foilCard = Common
		case 7:
			if ed.Name == "Time Spiral" {
				foilCard = Special
			}
		case 8:
			if ed.Name == "Vintage Masters" && r.Int32n(53) <= 7 {
				foilCard = Special
			}
		case 9:
			if t.hasSlot("dfc") {
				foilCard = Special
			}
		case 10:
			if ed.Name == "Planeshift" && r.Int32n(53) <= 3 {
				foilCard = Special
			}
		default:
			foilCard = Common
		}
	}

	extraFoilKey := ed.foilSheet
	replaceCommon := nSlots > 0 && r.Float64() < ed.replaceProb

	foilSlot := ""
	if hasFoil {
		pick := 0
		if nSlots > 1 {
			pick = int(r.Int32n(int32(nSlots)))
		}
		foilSlot = slotType(t.Slots[pick].spec)
		switch foilCard {
		case Rare:
			switch {
			case t.hasSlot("RareMythic"):
				foilSlot = "RareMythic"
			case t.hasSlot("Rare"):
				foilSlot = "Rare"
			case t.hasSlot("UncommonRare"):
				foilSlot = "UncommonRare"
			}
		case Uncommon:
			switch {
			case t.hasSlot("Uncommon"):
				foilSlot = "Uncommon"
			case t.hasSlot("UncommonRare"):
				foilSlot = "UncommonRare"
			}
		case Common:
			foilSlot = "Common"
			if t.hasSlot("BasicLand") && r.Int32n(130) <= 20 {
				foilSlot = "BasicLand"
			}
		case Special:
			switch {
			case t.hasSlot("TimeShifted"):
				foilSlot = "TimeShifted"
			case t.hasSlot("dfc"):
				foilSlot = "dfc"
			case t.hasSlot("Special"):
				foilSlot = "Special"
			}
		}
	}

	var held []*Print
	for _, slot := range t.Slots {
		spec := slot.spec
		numCards := slot.count
		convertFoil := strings.HasSuffix(spec, "+")
		spec = strings.TrimSuffix(spec, "+")

		setCode := ""
		if len(splitParen(spec, ' ', 1<<30)) == 1 {
			setCode = ed.Code
		}
		sheetKey := strings.TrimSpace(spec)
		if _, ok := g.t.byCode(setCode); ok {
			sheetKey = strings.TrimSpace(spec) + " " + setCode
		}
		if strings.HasPrefix(sheetKey, "wholeSheet") {
			ps, err := g.sheet(sheetKey)
			if err != nil {
				return nil, err
			}
			result = append(result, ps.flat()...)
			continue
		}

		typ := slotType(spec)
		foilInThisSlot := hasFoil && typ == foilSlot
		if (!foilAtEnd && foilInThisSlot) || (foilAtEnd && hasFoil && strings.HasPrefix(typ, "Common")) {
			numCards--
		}
		if ed.Name == "Planeshift" && strings.HasPrefix(typ, "Rare") && strings.HasPrefix(foilSlot, "Special") {
			numCards--
		}
		if replaceCommon && strings.HasPrefix(typ, "Common") {
			numCards--
			replaceKey := strings.TrimSpace(ed.replaceSlot)
			if _, ok := g.t.byCode(setCode); ok {
				replaceKey += " " + setCode
			}
			rs, err := g.sheet(replaceKey)
			if err != nil {
				return nil, err
			}
			drawn, err := rs.random(r, 1, true)
			if err != nil {
				return nil, err
			}
			result = append(result, drawn...)
			replaceCommon = false
		}

		ps, err := g.sheet(sheetKey)
		if err != nil {
			return nil, err
		}
		drawn, err := ps.random(r, numCards, true)
		if err != nil {
			return nil, err
		}
		for _, p := range drawn {
			if convertFoil {
				p = p.Foiled()
			}
			result = append(result, p)
		}

		if !foilInThisSlot {
			continue
		}
		if !foilAtEnd {
			hasFoil = false
			card, err := g.foilFrom(ps, extraFoilKey, r)
			if err != nil {
				return nil, err
			}
			result = appendIf(result, card)
			continue
		}
		switch {
		case extraFoilKey != "":
			card, err := g.foilFrom(ps, extraFoilKey, r)
			if err != nil {
				return nil, err
			}
			held = appendIf(held, card)
		case ed.Name == "Vintage Masters":
			if r.Int32n(30) == 1 || foilSlot != "Special" {
				card, err := g.foilFrom(ps, "", r)
				if err != nil {
					return nil, err
				}
				held = appendIf(held, card)
			} else {
				drawn, err := ps.random(r, 1, true)
				if err != nil {
					return nil, err
				}
				result = append(result, drawn...)
			}
		default:
			card, err := g.foilFrom(ps, "", r)
			if err != nil {
				return nil, err
			}
			held = appendIf(held, card)
		}
	}
	if hasFoil && foilAtEnd {
		result = append(result, held...)
	}

	var err error
	if ed.mustContain != "" {
		if result, err = g.ensureGuaranteed(result, t, ed.mustContain, r); err != nil {
			return nil, err
		}
	}
	if ed.replaceFrom != "" {
		if result, err = g.replaceFromExtraSheet(result, ed.replaceFrom, r); err != nil {
			return nil, err
		}
	}
	for _, spec := range []string{ed.sheetRepl1, ed.sheetRepl2} {
		if spec == "" {
			continue
		}
		first, second, _ := strings.Cut(spec, "_")
		replaceThis, err := g.t.staticSheet(first)
		if err != nil {
			return nil, err
		}
		if replaceThis == nil {
			return nil, fmt.Errorf("boosters: %s: sheet %q not found", ed.Code, first)
		}
		var candidates []*Print
		for _, p := range result {
			if replaceThis.contains(p) {
				candidates = append(candidates, p)
			}
		}
		result = removeAllEqual(result, candidates)
		if candidates, err = g.replaceFromExtraSheet(candidates, second, r); err != nil {
			return nil, err
		}
		result = append(result, candidates...)
	}
	return result, nil
}

func appendIf(list []*Print, p *Print) []*Print {
	if p == nil {
		return list
	}
	return append(list, p)
}

// foilFrom is generateFoilCard: from the sheet, or, with an extra sheet, from
// the sheet's distinct cards plus the extra sheet's of the same rarities,
// shuffled with the first taken.
func (g *Generator) foilFrom(ps *Sheet, extraKey string, r *javarand.Rand) (*Print, error) {
	if extraKey == "" {
		drawn, err := ps.random(r, 1, true)
		if err != nil || len(drawn) == 0 {
			return nil, err
		}
		return drawn[0].Foiled(), nil
	}
	var cards []*Print
	for _, p := range ps.flat() {
		if !containsEqual(cards, p) {
			cards = append(cards, p)
		}
	}
	extra, err := g.sheet(extraKey)
	if err != nil {
		return nil, err
	}
	allowed := map[Rarity]bool{}
	for _, p := range cards {
		allowed[p.Rarity] = true
	}
	for _, p := range extra.flat() {
		if !containsEqual(cards, p) && allowed[p.Rarity] {
			cards = append(cards, p)
		}
	}
	if len(cards) == 0 {
		return nil, nil
	}
	r.Shuffle(len(cards), func(i, j int) { cards[i], cards[j] = cards[j], cards[i] })
	return cards[0].Foiled(), nil
}

func containsEqual(list []*Print, p *Print) bool {
	for _, q := range list {
		if q.equal(p) {
			return true
		}
	}
	return false
}

func removeAllEqual(list, drop []*Print) []*Print {
	var out []*Print
	for _, p := range list {
		if !containsEqual(drop, p) {
			out = append(out, p)
		}
	}
	return out
}

// contains is PrintSheet.contains: the sheet holds an equal print.
func (s *Sheet) contains(p *Print) bool {
	for _, q := range s.prints {
		if q.equal(p) {
			return true
		}
	}
	return false
}

// replaceFromExtraSheet is replaceCardFromExtraSheet: one weighted card of the
// static sheet, not unique, replaces the first card of its rarity.
func (g *Generator) replaceFromExtraSheet(booster []*Print, key string, r *javarand.Rand) ([]*Print, error) {
	sheet, err := g.t.staticSheet(key)
	if err != nil {
		return nil, err
	}
	if sheet == nil {
		return booster, nil
	}
	picked, err := sheet.random(r, 1, false)
	if err != nil || len(picked) == 0 {
		return booster, err
	}
	return replaceCard(booster, picked[0]), nil
}

// replaceCard is BoosterGenerator.replaceCard: toAdd takes the place (the end of
// the list) of the first card of its rarity, rare and mythic alike, keeping a
// foil state; with none of that rarity nothing changes.
func replaceCard(booster []*Print, toAdd *Print) []*Print {
	var match func(*Print) bool
	switch toAdd.Rarity {
	case BasicLand:
		match = func(p *Print) bool { return p.Rarity == BasicLand }
	case Common:
		match = func(p *Print) bool { return p.Rarity == Common }
	case Uncommon:
		match = func(p *Print) bool { return p.Rarity == Uncommon }
	case Rare, MythicRare:
		match = func(p *Print) bool { return p.Rarity == Rare || p.Rarity == MythicRare }
	default:
		match = func(p *Print) bool { return p.Rarity == Special }
	}
	for i, p := range booster {
		if !match(p) {
			continue
		}
		if p.Foil {
			toAdd = toAdd.Foiled()
		}
		out := append(append([]*Print(nil), booster[:i]...), booster[i+1:]...)
		return append(out, toAdd)
	}
	return booster
}

// ensureGuaranteed is ensureGuaranteedCardInBooster: when no card of the pack
// has all the types, one chosen at random from the slots' sheets replaces one.
func (g *Generator) ensureGuaranteed(result []*Print, t *Template, mustContain string, r *javarand.Rand) ([]*Print, error) {
	types := strings.Fields(mustContain)
	hasAll := func(p *Print) bool {
		if p.rules == nil {
			return false
		}
		for _, typ := range types {
			if !p.rules.Faces[0].Type.HasStringType(typ) {
				return false
			}
		}
		return true
	}
	for _, p := range result {
		if hasAll(p) {
			return result, nil
		}
	}
	var possible []*Print
	for _, slot := range t.Slots {
		spec := strings.TrimSpace(slot.spec)
		key := spec
		if _, ok := g.t.byCode(t.Edition.Code); ok {
			key = spec + " " + t.Edition.Code
		}
		ps, err := g.sheet(key)
		if err != nil {
			return nil, err
		}
		for _, p := range ps.flat() {
			if hasAll(p) && !containsEqual(possible, p) {
				possible = append(possible, p)
			}
		}
	}
	if len(possible) == 0 {
		return result, nil
	}
	pick := possible[0]
	if len(possible) > 1 {
		pick = possible[r.Int32n(int32(len(possible)))]
	}
	return replaceCard(result, pick), nil
}

// Random draws n prints from the sheet as PrintSheet.random(n, wantUnique) does.
func (s *Sheet) Random(r *javarand.Rand, n int, wantUnique bool) ([]*Print, error) {
	return s.random(r, n, wantUnique)
}
