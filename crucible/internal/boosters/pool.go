// The card pool of Forge's CardDb (StaticData.getCommonCards): every print of
// every card, in the order CardDb.initialize adds them, with the lookups the
// booster code makes (ADR-0049).

package boosters

import (
	"math"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/jczastkiewicz/crucible/internal/carddb"
	"github.com/jczastkiewicz/crucible/internal/carddb/compile"
	"github.com/jczastkiewicz/crucible/internal/cardtype"
)

// unknownCode is CardEdition.UNKNOWN_CODE, the edition of a card no edition prints.
const unknownCode = "???"

// Print is a PaperCard: one printing of a card.
type Print struct {
	Name      string
	Edition   string
	Collector string
	Art       int
	Rarity    Rarity
	Foil      bool

	// flavor is the flavor name the entry used (a functional variant), "" when
	// the print is the card's own name.
	flavor string
	rules  *compile.Card
	land   bool
	basic  bool
	dfc    bool
	hash   int32
}

// hashOf is Objects.hash(name, edition, collectorNumber, artIndex, foil,
// flags) of PaperCard.hashCode, flags being the identity flags
// (PaperCardFlags.hashCode: Objects.hash(null, false)).
func (p *Print) hashOf() int32 {
	h := int32(1)
	h = 31*h + stringHash(p.Name)
	h = 31*h + stringHash(p.Edition)
	h = 31*h + stringHash(p.Collector)
	h = 31*h + int32(p.Art)
	foil := int32(1237)
	if p.Foil {
		foil = 1231
	}
	h = 31*h + foil
	const identityFlags = 31*(31*1+0) + 1237
	h = 31*h + identityFlags
	return h
}

// Foiled is PaperCard.getFoiled.
func (p *Print) Foiled() *Print {
	if p.Foil {
		return p
	}
	q := *p
	q.Foil = true
	q.hash = q.hashOf()
	return &q
}

func (p *Print) equal(q *Print) bool {
	return p.Name == q.Name && p.Edition == q.Edition && p.Collector == q.Collector && p.Art == q.Art && p.Foil == q.Foil
}

// Pool is the CardDb.
type Pool struct {
	editions []*Edition
	byCode   map[string]*Edition
	alias    map[string]*Edition
	// all is allCardsByRules.values(): rules in the order of their names
	// compared without case, each rules' prints in the order they were added.
	all []*Print
	// byName is allCardsByName, keys folded: every print under its name and,
	// for a card with several faces, under each face name.
	byName map[string][]*Print
	// primary is the prints of the rules whose own name this is.
	primary map[string][]*Print
}

func fold(s string) string { return strings.ToLower(s) }

// rulesName is CardRules.getName: the primary face name, "A // B" for a split card.
func rulesName(c *compile.Card) string {
	if c.SplitType == carddb.SplitSplit && c.Faces[carddb.FaceAlternate].Name != "" {
		return c.Faces[0].Name + " // " + c.Faces[carddb.FaceAlternate].Name
	}
	return c.Faces[0].Name
}

// rulesIsVariant is CardRules.isVariant: the card types Forge keeps in a
// separate database from the common cards (CardDb for StaticData.variantCards).
func rulesIsVariant(c *compile.Card) bool {
	for _, name := range []string{"Vanguard", "Scheme", "Plane", "Phenomenon", "Conspiracy", "Dungeon", "Attraction", "Contraption", "Stickers"} {
		if c.Faces[0].Type.HasStringType(name) {
			return true
		}
		if c.SplitType == carddb.SplitSplit && c.Faces[carddb.FaceAlternate].Type.HasStringType(name) {
			return true
		}
	}
	return false
}

func rulesIsBasicLand(c *compile.Card) bool {
	t := c.Faces[0].Type
	return t.Has(cardtype.Land) && t.HasSupertype(cardtype.Basic)
}

func rulesIsLand(c *compile.Card) bool {
	if c.Faces[0].Type.Has(cardtype.Land) {
		return true
	}
	return c.SplitType == carddb.SplitSplit && c.Faces[carddb.FaceAlternate].Type.Has(cardtype.Land)
}

// buildPool is CardDb.initialize over the editions: for each edition, newest
// first (date then name, reversed), each entry of the edition in cardsInSet
// order whose card exists becomes a print numbered by its name within the
// edition; cards no edition prints go to the UNKNOWN set as Special.
func buildPool(editions []*Edition, db *compile.DB, variant bool) (*Pool, error) {
	p := &Pool{editions: editions, byCode: map[string]*Edition{}, alias: map[string]*Edition{}, byName: map[string][]*Print{}, primary: map[string][]*Print{}}
	for _, e := range editions {
		p.byCode[fold(e.Code)] = e
	}
	// Collection.get: the code first, then the alias map (an edition's alias,
	// then its second code; a later edition overwrites an earlier alias).
	for _, e := range editions {
		if e.Alias != "" {
			p.alias[fold(e.Alias)] = e
		}
		p.alias[fold(e.Code2)] = e
	}

	var sorted []*compile.Card
	for _, c := range db.Cards() {
		if rulesIsVariant(c) == variant {
			sorted = append(sorted, c)
		}
	}
	sort.SliceStable(sorted, func(i, j int) bool { return fold(rulesName(sorted[i])) < fold(rulesName(sorted[j])) })
	rulesBy := map[string]*compile.Card{}
	altBy := map[string]*compile.Card{}
	for _, c := range sorted {
		rulesBy[fold(rulesName(c))] = c
	}
	for _, c := range sorted {
		for _, fn := range c.FlavorNames {
			altBy[fold(fn)] = c
		}
		for _, f := range c.Faces {
			if f.Name == "" {
				continue
			}
			if _, primary := rulesBy[fold(f.Name)]; !primary {
				if _, ok := altBy[fold(f.Name)]; !ok {
					altBy[fold(f.Name)] = c
				}
			}
		}
	}

	ordered := append([]*Edition(nil), editions...)
	sort.SliceStable(ordered, func(i, j int) bool {
		a, b := ordered[i], ordered[j]
		if !a.Date.Equal(b.Date) {
			return a.Date.After(b.Date)
		}
		return a.Name > b.Name
	})

	byRules := map[*compile.Card][]*Print{}
	var order []*compile.Card
	add := func(c *compile.Card, pr *Print) {
		if _, ok := byRules[c]; !ok {
			order = append(order, c)
		}
		byRules[c] = append(byRules[c], pr)
		p.byName[fold(pr.Name)] = append(p.byName[fold(pr.Name)], pr)
		p.primary[fold(pr.Name)] = append(p.primary[fold(pr.Name)], pr)
		if pr.flavor != "" {
			p.byName[fold(pr.flavor)] = append(p.byName[fold(pr.flavor)], pr)
		}
		if c.SplitType != carddb.SplitNone {
			seen := map[string]bool{fold(pr.Name): true}
			for _, f := range c.Faces {
				if f.Name != "" && !seen[fold(f.Name)] {
					seen[fold(f.Name)] = true
					p.byName[fold(f.Name)] = append(p.byName[fold(f.Name)], pr)
				}
			}
		}
	}
	for _, e := range ordered {
		artIDs := map[string]int{}
		for _, en := range e.cards {
			c := rulesBy[fold(en.name)]
			if c == nil {
				c = altBy[fold(en.name)]
			}
			if c == nil {
				continue
			}
			if len(c.SupportedVariants) > 0 && en.variant != "" && !slices.Contains(c.SupportedVariants, en.variant) {
				// Supported card, unsupported variant (CardDb.initialize).
				continue
			}
			art := 1
			key := e.Code + "/" + en.name
			if n, ok := artIDs[key]; ok {
				art = n + 1
			}
			artIDs[key] = art
			coll := en.collector
			if coll == "" {
				coll = noCollectorNumber
			}
			flavor := ""
			if !strings.EqualFold(en.name, rulesName(c)) && slices.ContainsFunc(c.FlavorNames, func(f string) bool { return strings.EqualFold(f, en.name) }) {
				flavor = en.name
			}
			pr := &Print{flavor: flavor, rules: c, Name: rulesName(c), Edition: e.Code, Collector: coll, Art: art, Rarity: en.rarity,
				land: rulesIsLand(c), basic: rulesIsBasicLand(c), dfc: c.SplitType == carddb.SplitTransform || c.SplitType == carddb.SplitMeld || c.SplitType == carddb.SplitModal}
			pr.hash = pr.hashOf()
			add(c, pr)
		}
	}
	for _, c := range sorted {
		if _, ok := byRules[c]; ok {
			continue
		}
		pr := &Print{rules: c, Name: rulesName(c), Edition: unknownCode, Collector: noCollectorNumber, Art: 1, Rarity: Special,
			land: rulesIsLand(c), basic: rulesIsBasicLand(c), dfc: c.SplitType == carddb.SplitTransform || c.SplitType == carddb.SplitMeld || c.SplitType == carddb.SplitModal}
		pr.hash = pr.hashOf()
		add(c, pr)
	}
	sort.SliceStable(order, func(i, j int) bool { return fold(rulesName(order[i])) < fold(rulesName(order[j])) })
	for _, c := range order {
		p.all = append(p.all, byRules[c]...)
	}
	return p, nil
}

// request is CardDb.CardRequest for the "Name|SET|art|[collector]" strings
// sheets name cards by.
type request struct {
	name      string
	edition   string
	art       int
	collector string
}

func parseRequest(s string) request {
	parts := strings.Split(s, "|")
	r := request{name: parts[0], art: -1, collector: noCollectorNumber}
	i := 1
	isNumeric := func(x string) bool {
		if x == "" {
			return false
		}
		for _, c := range x {
			if c < '0' || c > '9' {
				return false
			}
		}
		return true
	}
	if len(parts) > i && !isNumeric(parts[i]) {
		r.edition = parts[i]
		i++
	}
	if len(parts) > i && isNumeric(parts[i]) && len(parts[i]) <= 2 {
		r.art, _ = strconv.Atoi(parts[i])
		i++
	}
	if len(parts) > i && strings.HasPrefix(parts[i], "[") && strings.HasSuffix(parts[i], "]") {
		r.collector = parts[i][1 : len(parts[i])-1]
	}
	if r.collector == noCollectorNumber && r.art == -1 {
		r.art = 1
	}
	return r
}

// getCard is CardDb.getCard for a request string. A request that names no
// edition, or one the card is not printed in, falls to Forge's art
// preference, which this port does not model: that is an error.
func (p *Pool) getCard(req string) (*Print, error) {
	r := parseRequest(req)
	if r.edition != "" {
		if e := p.edition(r.edition); e != nil {
			if pr := p.fromSet(r.name, e, r.art, r.collector); pr != nil {
				return pr, nil
			}
		}
	}
	// CardDb.tryGetCard step 2: any print of the name, by the card art preference.
	if len(p.byName[fold(r.name)]) == 0 {
		return nil, nil
	}
	return p.fromEditions(r.name, r.art), nil
}

// fromEditions is CardDb.tryToGetCardFromEditions with LATEST_ART_ALL_EDITIONS
// (every edition accepted, newest first), the default of a game: the print of
// the name with this art index from the newest edition, in its traditional
// frame.
func (p *Pool) fromEditions(name string, art int) *Print {
	art = max(art, 1)
	pick := func(prints []*Print) []*Print {
		var out []*Print
		for _, pr := range prints {
			if pr.Art == art {
				out = append(out, pr)
			}
		}
		return out
	}
	cards := pick(p.primary[fold(name)])
	if len(cards) == 0 {
		cards = pick(p.byName[fold(name)])
	}
	if len(cards) == 0 {
		return nil
	}
	if len(cards) == 1 {
		return p.traditionalFramePrint(cards[0])
	}
	type ed struct {
		code string
		date time.Time
		name string
	}
	var eds []ed
	byCode := map[string]*Print{}
	for _, c := range cards {
		var e ed
		if c.Edition == unknownCode {
			e = ed{code: unknownCode, date: time.Now(), name: unknownCode}
		} else if x := p.edition(c.Edition); x != nil {
			e = ed{code: x.Code, date: x.Date, name: x.Name}
		} else {
			continue
		}
		eds = append(eds, e)
		byCode[c.Edition] = c
	}
	if len(eds) == 0 {
		return nil
	}
	sort.SliceStable(eds, func(i, j int) bool {
		if !eds[i].date.Equal(eds[j].date) {
			return eds[i].date.Before(eds[j].date)
		}
		return eds[i].name < eds[j].name
	})
	best := eds[len(eds)-1]
	return p.traditionalFramePrint(byCode[best.code])
}

// traditionalFramePrint is CardDb.getTraditionalFramePrint: a special print
// (borderless, showcase, ...) of a card the main "cards" section also lists
// becomes that print.
func (p *Pool) traditionalFramePrint(pr *Print) *Print {
	if p.isTraditionalFramePrint(pr) {
		return pr
	}
	trad := p.traditionalPrints(pr)
	if len(trad) == 0 {
		return pr
	}
	return trad[(max(pr.Art, 1)-1)%len(trad)]
}

func (e *Edition) isTraditionalFrame(collector string) bool {
	if collector == "" {
		return false
	}
	return e.traditional[strings.ToLower(collector)]
}

func (p *Pool) isTraditionalFramePrint(pr *Print) bool {
	e := p.edition(pr.Edition)
	if e == nil || !e.isTraditionalFrame(pr.Collector) {
		return false
	}
	if pr.basic {
		return true
	}
	for _, t := range p.traditionalPrints(pr) {
		if strings.EqualFold(t.Collector, pr.Collector) {
			return true
		}
	}
	return false
}

// traditionalPrints is CardDb.getTraditionalFramePrints: the prints of the
// same card in the edition whose collector number the "cards" section lists,
// by art index; a card that is not a basic land keeps only the lowest number.
func (p *Pool) traditionalPrints(pr *Print) []*Print {
	e := p.edition(pr.Edition)
	if e == nil {
		return nil
	}
	var out []*Print
	for _, o := range p.primary[fold(pr.Name)] {
		if o.Edition == pr.Edition && e.isTraditionalFrame(o.Collector) {
			out = append(out, o)
		}
	}
	if !pr.basic {
		lowest := math.MaxInt32
		for _, o := range out {
			lowest = min(lowest, collectorBase(o.Collector))
		}
		kept := out[:0:0]
		for _, o := range out {
			if n := collectorBase(o.Collector); n == math.MaxInt32 || n == lowest {
				kept = append(kept, o)
			}
		}
		out = kept
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Art < out[j].Art })
	return out
}

// collectorBase is CardDb.collectorNumberBase: the leading number of a collector
// number (at most nine digits), MaxInt32 when it has none.
func collectorBase(c string) int {
	end := 0
	for end < len(c) && end < 9 && c[end] >= '0' && c[end] <= '9' {
		end++
	}
	if end == 0 {
		return math.MaxInt32
	}
	n, _ := strconv.Atoi(c[:end])
	return n
}

// fromSet is CardDb.getCardFromSet: the first print of the name in the
// edition (either of its codes) with this art and collector number.
func (p *Pool) fromSet(name string, e *Edition, art int, collector string) *Print {
	var out []*Print
	for _, pr := range p.byName[fold(name)] {
		if !strings.EqualFold(pr.Edition, e.Code) && !strings.EqualFold(pr.Edition, e.Code2) {
			continue
		}
		if art > 0 && pr.Art != art {
			continue
		}
		if collector != "" && collector != noCollectorNumber && pr.Collector != collector {
			continue
		}
		out = append(out, pr)
	}
	if len(out) == 0 {
		return nil
	}
	// Several rules (an alternate face shared by two cards): the exact name wins.
	distinct := map[string]bool{}
	exact := false
	for _, pr := range out {
		distinct[pr.Name] = true
		if strings.EqualFold(pr.Name, name) {
			exact = true
		}
	}
	if len(distinct) > 1 && exact {
		kept := out[:0:0]
		for _, pr := range out {
			if strings.EqualFold(pr.Name, name) {
				kept = append(kept, pr)
			}
		}
		out = kept
	}
	return out[0]
}

// edition is CardEdition.Collection.get: by code, else by alias or second code.
func (p *Pool) edition(code string) *Edition {
	if e := p.byCode[fold(code)]; e != nil {
		return e
	}
	return p.alias[fold(code)]
}

// unsupported is CardDb.createUnsupportedCard for a request: a print of a card
// with no rules, so that no rarity filter but its own edition's can pass it.
func (p *Pool) unsupported(req string) *Print {
	r := parseRequest(req)
	edition, rarity := unknownCode, Unknown
	if r.edition == "" {
		for _, e := range p.editions {
			for _, en := range e.cards {
				if en.name == r.name {
					edition, rarity = e.Code, en.rarity
					break
				}
			}
			if edition != unknownCode {
				break
			}
		}
	} else if e := p.edition(r.edition); e != nil {
		edition = e.Code
		for _, en := range e.cards {
			if en.name == r.name {
				rarity = en.rarity
				break
			}
		}
	}
	pr := &Print{Name: r.name, Edition: edition, Collector: noCollectorNumber, Art: 1, Rarity: rarity}
	pr.hash = pr.hashOf()
	return pr
}
