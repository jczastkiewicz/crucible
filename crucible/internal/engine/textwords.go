package engine

// Layer 3 word substitution (CR 612, ADR-0039): the records ChangeText and
// ExchangeTextBox leave on a Game, the ChangeColorWordsTo$ statics, and the
// stage that folds them into one rewritten Def per card. textrewrite.go holds
// the pure half.
//
// Ported from forge-game/.../card/Card.java (addChangedTextColorWord,
// addChangedTextTypeWord, addChangedCardTraitsByText, updateChangedText),
// staticability/StaticAbilityContinuous.java:660-675 (ChangeColorWordsTo$) and
// ability/effects/TextBoxExchangeEffect.java (captureTextBoxData, swapTextBox).

import (
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/jczastkiewicz/crucible/internal/carddb/compile"
	"github.com/jczastkiewicz/crucible/internal/expr"
	"github.com/jczastkiewicz/crucible/internal/mana"
)

// wordKind is which of Card.changedTextColors and changedTextTypes a record
// writes.
type wordKind uint8

const (
	wordColor wordKind = iota
	wordType
)

// textWordRecord is one resolved ChangeText word change on one card
// (Card.addChangedTextColorWord/TypeWord). Permanent is Duration$ Permanent;
// any other value ends at cleanup (ChangeTextEffect.java:29).
type textWordRecord struct {
	Card      CardID
	Timestamp uint64
	Kind      wordKind
	From, To  string
	Permanent bool
}

// textBoxRecord is one card's side of a resolved ExchangeTextBox: Source is
// the other card's intrinsic text box as it stood at resolution
// (captureTextBoxData), its own word changes already baked in. A record
// bound to a host (Duration$ AsLongAsInPlay, UntilHostLeavesPlay) ends when
// the host's object leaves the battlefield, or phases out for AsLongAsInPlay.
type textBoxRecord struct {
	Card      CardID
	Timestamp uint64
	Source    compile.Face

	hostBound  bool
	phaseEnds  bool
	host       CardID
	hostObject uint64
}

// textMemoKey names one rewritten definition: the definition under the change,
// the exchange it sits on (zero for none) and the folded word map.
type textMemoKey struct {
	def     *compile.Card
	box     uint64
	boxCard CardID
	words   string
}

// textRow is one timestamped word row of one card, before folding.
type textRow struct {
	ts   uint64
	id   int
	kind wordKind
	row  wordRow
}

// colorWordFor is a color word as Card.addChangedTextColorWord stores it:
// capitalized, "Any" kept, anything that is not a color refused (Java throws
// "Not a color").
func colorWordFor(name string) (string, bool) {
	if name == "Any" {
		return name, true
	}
	word := capitalizeWord(strings.ToLower(name))
	if _, ok := colorFromName(word); !ok {
		return "", false
	}
	return word, true
}

// colorWordOf is the capitalized long name of a single color.
func colorWordOf(c mana.Colors) (string, bool) {
	for _, e := range layerColorNames {
		if c == e.color {
			return capitalizeWord(e.name), true
		}
	}
	return "", false
}

// addTextWord records a word change on id (Card.addChangedTextColorWord /
// addChangedTextTypeWord).
func (g *Game) addTextWord(r textWordRecord) { g.textWords = append(g.textWords, r) }

// clearTextRecords ends every word change and text-box exchange on id: a
// card that changes zone is a new object (CR 400.7), so its text changes end.
func (g *Game) clearTextRecords(id CardID) {
	if len(g.textWords) > 0 {
		kept := g.textWords[:0:0]
		for _, r := range g.textWords {
			if r.Card != id {
				kept = append(kept, r)
			}
		}
		g.textWords = kept
	}
	if len(g.textBoxes) > 0 {
		kept := g.textBoxes[:0:0]
		for _, r := range g.textBoxes {
			if r.Card != id {
				kept = append(kept, r)
			}
		}
		g.textBoxes = kept
	}
}

// endTextAtCleanup drops every word change that was not Duration$ Permanent:
// CR 514.2's "until end of turn" ending (ChangeTextEffect's revert command on
// the end-of-turn list).
func (g *Game) endTextAtCleanup() {
	kept := g.textWords[:0:0]
	for _, r := range g.textWords {
		if r.Permanent {
			kept = append(kept, r)
		}
	}
	g.textWords = kept
}

// dropEndedTextBoxes removes the host-bound exchanges whose host object left
// (the leaves-play and phase-out commands addUntilCommand registers).
func (g *Game) dropEndedTextBoxes() {
	if len(g.textBoxes) == 0 {
		return
	}
	kept := g.textBoxes[:0:0]
	for _, b := range g.textBoxes {
		if b.hostBound {
			h := g.Card(b.host)
			if h.Zone != Battlefield || h.zoneStamp != b.hostObject || b.phaseEnds && h.IsPhasedOut() {
				continue
			}
		}
		kept = append(kept, b)
	}
	g.textBoxes = kept
}

// staticWordRows reads every live Mode$ Continuous line naming
// ChangeColorWordsTo$ (StaticAbilityContinuous.java:660-675): ChosenColor is
// the host's chosen color (nothing while none is chosen), anything else a
// color name; ChangeColorWordsFrom$ defaults to Any. Each affected card gets
// one row at the host's timestamp.
func (g *Game) staticWordRows(add func(CardID, textRow)) {
	n := 0
	for _, ls := range continuousStatics(g) {
		s := ls.s
		if !strings.EqualFold(s.Name, "Continuous") {
			continue
		}
		to, ok := s.Param("ChangeColorWordsTo")
		if !ok {
			continue
		}
		host := g.Card(ls.host)
		if !g.staticLive(host, s) || !layerStaticApplies(g, host, ls.amounts, s) {
			continue
		}
		word := ""
		if to == "ChosenColor" {
			c := host.Memory.ChosenColors()
			if c.Count() != 1 {
				continue
			}
			word, ok = colorWordOf(c)
		} else {
			word, ok = colorWordFor(to)
		}
		if !ok {
			continue
		}
		from := "Any"
		if v, ok := s.Param("ChangeColorWordsFrom"); ok {
			if from, ok = colorWordFor(v); !ok {
				continue
			}
		}
		ids, ok := g.staticAffected(host, s)
		if !ok {
			continue
		}
		n++
		for _, id := range ids {
			add(id, textRow{ts: host.Timestamp, id: -n, kind: wordColor, row: wordRow{from: from, to: word}})
		}
	}
}

// applyTextWords is the word-substitution half of Layer 3, run after
// GainTextOf$ (applyContinuousText): every card with a word change, an
// exchanged text box or a ChangeColorWordsTo$ line affecting it gets its Def
// rewritten (ADR-0039), and g.textMaps keeps the folded map for spells on the
// stack and anything else that reads the words.
func applyTextWords(g *Game) {
	g.dropEndedTextBoxes()
	g.textMaps = nil
	type staticRow struct {
		id  CardID
		row textRow
	}
	var fromStatics []staticRow
	g.staticWordRows(func(id CardID, r textRow) { fromStatics = append(fromStatics, staticRow{id, r}) })
	if len(g.textWords) == 0 && len(g.textBoxes) == 0 && len(fromStatics) == 0 {
		return
	}
	rows := map[CardID][]textRow{}
	var order []CardID
	add := func(id CardID, r textRow) {
		if _, ok := rows[id]; !ok {
			order = append(order, id)
		}
		rows[id] = append(rows[id], r)
	}
	for i, r := range g.textWords {
		add(r.Card, textRow{ts: r.Timestamp, id: i, kind: r.Kind, row: wordRow{from: r.From, to: r.To}})
	}
	boxes := map[CardID]*textBoxRecord{}
	for i := range g.textBoxes {
		b := &g.textBoxes[i]
		// addChangedCardTraitsByText: the exchange clears every earlier word
		// change on both tables (Card.java:4895-4896).
		add(b.Card, textRow{ts: b.Timestamp, id: i, row: wordRow{clear: true}})
		if cur := boxes[b.Card]; cur == nil || b.Timestamp >= cur.Timestamp {
			boxes[b.Card] = b
		}
	}
	for _, sr := range fromStatics {
		add(sr.id, sr.row)
	}

	for _, id := range order {
		c := g.Card(id)
		if c.Def == nil {
			continue
		}
		rs := rows[id]
		sort.SliceStable(rs, func(i, j int) bool {
			if rs[i].ts != rs[j].ts {
				return rs[i].ts < rs[j].ts
			}
			return rs[i].id < rs[j].id
		})
		var colorRows, typeRows []wordRow
		for _, r := range rs {
			if r.row.clear || r.kind == wordColor {
				colorRows = append(colorRows, r.row)
			}
			if r.row.clear || r.kind == wordType {
				typeRows = append(typeRows, r.row)
			}
		}
		wm := wordMap{colors: foldWords(colorRows), types: foldWords(typeRows)}
		box := boxes[id]
		if wm.empty() && box == nil {
			continue
		}
		if !wm.empty() {
			if g.textMaps == nil {
				g.textMaps = map[CardID]wordMap{}
			}
			g.textMaps[id] = wm
		}
		c.setTextChange(textChange{def: g.textDef(c.Def, box, wm)})
	}
}

// textDef is the definition def has under box's exchange and word map wm,
// memoised for the game (ADR-0039). Never shared across games: a Game.Clone
// starts with an empty memo.
func (g *Game) textDef(def *compile.Card, box *textBoxRecord, wm wordMap) *compile.Card {
	key := textMemoKey{def: def, words: wm.key()}
	if box != nil {
		key.box, key.boxCard = box.Timestamp, box.Card
	}
	if d, ok := g.textMemo[key]; ok {
		return d
	}
	base := def
	if box != nil {
		base = exchangedDef(def, box.Source)
	}
	out := base
	if !wm.empty() {
		out = rewriteDef(base, wm)
	}
	if g.textMemo == nil {
		g.textMemo = map[textMemoKey]*compile.Card{}
	}
	g.textMemo[key] = out
	return out
}

// exchangedDef is def with its text box replaced by src's
// (Card.addChangedCardTraitsByText's remove-everything predicate, then the
// copied traits, and addChangedCardKeywordsByText's keywords): abilities,
// triggers, statics, replacement effects, keywords and the SVar amounts they
// read. The name, cost, color, types and power/toughness stay. The merged
// amounts table is a fresh map: src's and def's are shared by every game.
func exchangedDef(def *compile.Card, src compile.Face) *compile.Card {
	out := *def
	f := def.Faces[0]
	f.Abilities, f.Triggers, f.Statics, f.Replacements = src.Abilities, src.Triggers, src.Statics, src.Replacements
	f.Keywords = src.Keywords
	if len(src.Amounts) > 0 {
		merged := make(map[string]expr.Amount, len(f.Amounts)+len(src.Amounts))
		for k, v := range f.Amounts {
			merged[k] = v
		}
		for k, v := range src.Amounts {
			merged[k] = v
		}
		f.Amounts = merged
	}
	out.Faces[0] = f
	return &out
}

// costSubstitutedGrant is the per-card text substitution a Layer 6 grant makes
// before it parses an AddAbility$ or AddStaticAbility$ body
// (StaticAbilityContinuous.java:777-784, 842-847): CardManaCost becomes the
// affected card's short mana cost string (ManaCost.getShortString) and, only
// when the body has no CardManaCost, ConvertedManaCost becomes its mana value;
// a static takes the second only. The substitution reaches the granted line's
// own params, not the SVars it chains to, which Java parses unchanged. Bodies
// that name neither token are shared as is, so a grant without them allocates
// nothing.
func costSubstitutedGrant(grant traitGrant, c *Card) traitGrant {
	if c.Def == nil {
		return grant
	}
	short := c.Def.Faces[0].ManaCost.ShortString()
	cmc := strconv.Itoa(c.CMC())
	out := grant
	out.abilities = substituteBodies(grant.abilities, short, cmc)
	out.statics = substituteBodies(grant.statics, "", cmc)
	return out
}

// substituteBodies returns list with each body's params rewritten by
// substituteBody, the list itself when none changed.
func substituteBodies(list []*compile.Ability, short, cmc string) []*compile.Ability {
	var out []*compile.Ability
	for i, a := range list {
		sub := substituteBody(a, short, cmc)
		if sub == a {
			continue
		}
		if out == nil {
			out = slices.Clone(list)
		}
		out[i] = sub
	}
	if out == nil {
		return list
	}
	return out
}

// substituteBody is one granted body's token replacement; a is returned
// unchanged when it holds no token. short is "" for a static, which never
// reads CardManaCost.
func substituteBody(a *compile.Ability, short, cmc string) *compile.Ability {
	hasCost, hasCMC := false, false
	for _, p := range a.Params {
		hasCost = hasCost || short != "" && strings.Contains(p.Value, "CardManaCost")
		hasCMC = hasCMC || strings.Contains(p.Value, "ConvertedManaCost")
	}
	if !hasCost && !hasCMC {
		return a
	}
	token, with := "ConvertedManaCost", cmc
	if hasCost {
		token, with = "CardManaCost", short
	}
	out := *a
	out.Params = slices.Clone(a.Params)
	for i := range out.Params {
		out.Params[i].Value = strings.ReplaceAll(out.Params[i].Value, token, with)
	}
	return &out
}

// textChangedSpell is the stack half of Layer 3: a spell resolves from the
// ability built when it was cast, so a word change that reached its card
// since rewrites that ability's params as it resolves
// (MagicStack.java:580, spells only). The ability is copied; the original
// stays on the stack record.
func (g *Game) textChangedSpell(a *Ability) *Ability {
	if !a.spell || a.Params == nil {
		return a
	}
	wm, ok := g.textMaps[a.Source]
	if !ok {
		return a
	}
	cp := *a
	cp.Params = rewriteAbility(a.Params, func(s string) string { return rewriteText(s, wm) },
		amountNames(a.Amounts), map[*compile.Ability]*compile.Ability{})
	return &cp
}
