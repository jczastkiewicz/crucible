package engine

// Layer 3 word substitution (CR 612), the pure half: the folded word map
// CardChangedWords holds and the rewrite of a compiled definition under it
// (ADR-0039). Nothing here reads a Game; textwords.go owns the records and
// the layer stage.
//
// Ported from forge-game/.../card/CardChangedWords.java (refreshCache),
// ability/AbilityUtils.java:3013-3099 (applyTextChangeEffects,
// getReplacedText), CardTraitBase.java:691-712 (changeText),
// card/CardUtil.java:53-76 (isKeywordModifiable) and
// forge-core/.../card/WordChangedType.java (applyChanges).

import (
	"strings"

	"github.com/jczastkiewicz/crucible/internal/carddb/compile"
	"github.com/jczastkiewicz/crucible/internal/carddb/vocab"
	"github.com/jczastkiewicz/crucible/internal/cardtype"
	"github.com/jczastkiewicz/crucible/internal/expr"
)

// wordPair is one entry of a folded word table: replace From with To.
type wordPair struct{ from, to string }

// wordMap is the two flat tables Card.getChangedTextColorWords and
// getChangedTextTypeWords return: insertion-ordered, one entry per source
// word.
type wordMap struct {
	colors []wordPair
	types  []wordPair
}

// empty reports whether m changes no word.
func (m wordMap) empty() bool { return len(m.colors) == 0 && len(m.types) == 0 }

// key is a stable text form of m, the memo key for a rewritten definition.
func (m wordMap) key() string {
	var b strings.Builder
	for _, p := range m.colors {
		b.WriteString("c:" + p.from + ">" + p.to + ";")
	}
	for _, p := range m.types {
		b.WriteString("t:" + p.from + ">" + p.to + ";")
	}
	return b.String()
}

// wordRow is one timestamped row of a CardChangedWords table: from becomes
// to, or, when clear is set, every older row is dropped (addEmpty).
type wordRow struct {
	from, to string
	clear    bool
}

// foldWords is CardChangedWords.refreshCache: rows in timestamp order. A
// clear row empties the table; any other row first rewrites every existing
// entry whose value is its from word (so a->b then b->c leaves a->c), then
// puts from->to, keeping the position of an existing key.
func foldWords(rows []wordRow) []wordPair {
	var out []wordPair
	for _, r := range rows {
		if r.clear {
			out = out[:0:0]
			continue
		}
		for i := range out {
			if out[i].to == r.from {
				out[i].to = r.to
			}
		}
		found := false
		for i := range out {
			if out[i].from == r.from {
				out[i].to, found = r.to, true
				break
			}
		}
		if !found {
			out = append(out, wordPair{r.from, r.to})
		}
	}
	return out
}

// colorWordNames are MagicColor.WUBRG's long names, the "Any" source's
// expansion.
var colorWordNames = [...]string{"White", "Blue", "Black", "Red", "Green"}

// capitalizeWord is StringUtils.capitalize.
func capitalizeWord(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

func isWordByte(b byte) bool {
	return b == '_' || b >= '0' && b <= '9' || b >= 'a' && b <= 'z' || b >= 'A' && b <= 'Z'
}

// wordBoundaryAt is regex \b at byte offset i.
func wordBoundaryAt(s string, i int) bool {
	before := i > 0 && isWordByte(s[i-1])
	after := i < len(s) && isWordByte(s[i])
	return before != after
}

// followsNamed is the lookbehind (?<!named.{0,100}) failing at i: "named"
// ends at most 100 characters before i.
func followsNamed(s string, i int) bool {
	for j := i - 5; j >= 0 && i-(j+5) <= 100; j-- {
		if s[j:j+5] == "named" {
			return true
		}
	}
	return false
}

// replaceWord is AbilityUtils.getReplacedText for a non-descriptive text:
// text.replaceAll("(?<!named.{0,100})\\b(non)?" + orig, "$1" + repl). There is
// a left word boundary and no right one, so a color or type word also
// matches the front of a longer word (Swampwalk).
func replaceWord(s, orig, repl string) string {
	if orig == "" || !strings.Contains(s, orig) {
		return s
	}
	var b strings.Builder
	for i := 0; i < len(s); {
		if wordBoundaryAt(s, i) && !followsNamed(s, i) {
			if rest := s[i:]; strings.HasPrefix(rest, "non"+orig) {
				b.WriteString("non" + repl)
				i += 3 + len(orig)
				continue
			} else if strings.HasPrefix(rest, orig) {
				b.WriteString(repl)
				i += len(orig)
				continue
			}
		}
		b.WriteByte(s[i])
		i++
	}
	return b.String()
}

// rewriteText is AbilityUtils.applyTextChangeEffects (isDescriptive false):
// every color entry, in order, lower case then as written, "Any" standing for
// each color but its own destination; then every type entry as written.
func rewriteText(s string, m wordMap) string {
	if s == "" {
		return s
	}
	for _, e := range m.colors {
		if e.from == "Any" {
			for _, c := range colorWordNames {
				if strings.EqualFold(e.to, c) {
					continue
				}
				s = replaceWord(s, strings.ToLower(c), strings.ToLower(e.to))
				s = replaceWord(s, c, e.to)
			}
			continue
		}
		s = replaceWord(s, strings.ToLower(e.from), strings.ToLower(e.to))
		s = replaceWord(s, e.from, e.to)
	}
	for _, e := range m.types {
		s = replaceWord(s, e.from, e.to)
	}
	return s
}

// modifiableKeywords is CardUtil.modifiableKeywords: the keywords a text
// change can reach, matched by prefix.
var modifiableKeywords = [...]string{
	"Enchant", "Protection", "Cumulative upkeep", "Equip", "Buyback",
	"Cycling", "Echo", "Kicker", "Flashback", "Madness", "Morph",
	"Affinity", "Entwine", "Splice", "Ninjutsu",
	"Transmute", "Replicate", "Recover", "Squad", "Suspend", "Aura swap",
	"Fortify", "Transfigure", "Champion", "Evoke", "Prowl", "Freerunning",
	"Reinforce", "Unearth", "Level up", "Miracle", "Overload", "Cleave",
	"Scavenge", "Encore", "Bestow", "Outlast", "Dash", "Surge", "Emerge", "Hexproof:",
	"Bands with other", "Landwalk", "Offering",
	"etbCounter", "Reflect", "Ward",
}

// keywordModifiable is CardUtil.isKeywordModifiable.
func keywordModifiable(kw string) bool {
	for _, p := range modifiableKeywords {
		if strings.HasPrefix(kw, p) {
			return true
		}
	}
	return false
}

// textNoChangeKeys are the params CardTraitBase.noChangeKeys leaves alone;
// textDescriptiveKeys are the ones Java rewrites in strike-out style, which
// this port, holding no rendered text, leaves as written.
var (
	textNoChangeKeys = [...]string{"TokenScript", "NewName", "DefinedName", "ChooseFromList", "AddAbility"}
	textDescriptive  = [...]string{
		"Description", "SpellDescription", "StackDescription", "TriggerDescription", "ChangeTypeDesc", "ValidTgtsDesc",
	}
)

func keyIn(key string, list []string) bool {
	for _, k := range list {
		if strings.EqualFold(k, key) {
			return true
		}
	}
	return false
}

// rewriteAbility is CardTraitBase.changeText for one compiled trait and its
// sub-abilities: every param value is rewritten except the no-change keys,
// descriptions, and a value that names an SVar (a sub-ability or an amount of
// amounts). A trait carrying LockInText$ is returned as is. memo keeps a
// sub-ability shared by two parents shared in the copy. The result shares
// nothing mutable with a, which every game in the process reads.
func rewriteAbility(a *compile.Ability, fn func(string) string, amounts map[string]struct{}, memo map[*compile.Ability]*compile.Ability) *compile.Ability {
	if done, ok := memo[a]; ok {
		return done
	}
	if _, locked := a.Param("LockInText"); locked {
		memo[a] = a
		return a
	}
	out := *a
	subNames := make(map[string]struct{}, len(a.Subs))
	for _, s := range a.Subs {
		subNames[s.SVar] = struct{}{}
	}
	out.Params = make([]vocab.Param, len(a.Params))
	for i, p := range a.Params {
		out.Params[i] = p
		if keyIn(p.Key, textNoChangeKeys[:]) || keyIn(p.Key, textDescriptive[:]) {
			continue
		}
		if _, ok := subNames[p.Value]; ok {
			continue
		}
		if _, ok := amounts[strings.ToLower(p.Value)]; ok {
			continue
		}
		out.Params[i].Value = fn(p.Value)
	}
	memo[a] = &out
	out.Subs = make([]compile.SubRef, len(a.Subs))
	for i, s := range a.Subs {
		out.Subs[i] = s
		out.Subs[i].Ability = rewriteAbility(s.Ability, fn, amounts, memo)
	}
	return &out
}

func rewriteAbilities(list []*compile.Ability, fn func(string) string, amounts map[string]struct{}, memo map[*compile.Ability]*compile.Ability) []*compile.Ability {
	if list == nil {
		return nil
	}
	out := make([]*compile.Ability, len(list))
	for i, a := range list {
		out[i] = rewriteAbility(a, fn, amounts, memo)
	}
	return out
}

// amountNames is the set of SVar names a face's amounts table holds, folded to
// lower case as the table itself is.
func amountNames(amounts map[string]expr.Amount) map[string]struct{} {
	out := make(map[string]struct{}, len(amounts))
	for k := range amounts {
		out[k] = struct{}{}
	}
	return out
}

// rewriteType is WordChangedType.applyChanges over a type line: a type that
// has the source word as a subtype (or other string type) gives it up for the
// destination, in table order.
func rewriteType(l cardtype.Line, m wordMap) cardtype.Line {
	for _, e := range m.types {
		if l.HasStringType(e.from) {
			l = l.Without(cardtype.ParseToken(e.from)).Union(cardtype.ParseToken(e.to))
		}
	}
	return l
}

// rewriteAmounts is AbilityUtils.calculateAmount's `calcX[1] =
// applyAbilityTextChangeEffects(calcX[1], ability)` (:440) done once per
// definition: the part of an amount after its first `$` ("Valid Creature.Elf"
// of `Count$Valid Creature.Elf`) is rewritten and the amount read again from
// the new text. A Number$ amount is read from its whole, unrewritten text
// (:447-448). The table is copied only when some amount changes; the keys,
// which an inline amount takes from its own text, stay as written.
func rewriteAmounts(amounts map[string]expr.Amount, fn func(string) string) map[string]expr.Amount {
	var out map[string]expr.Amount
	for name, a := range amounts {
		head, rest, ok := strings.Cut(a.Text, "$")
		if !ok || strings.HasPrefix(head, "Number") {
			continue
		}
		changed := fn(rest)
		if changed == rest {
			continue
		}
		if out == nil {
			out = make(map[string]expr.Amount, len(amounts))
			for k, v := range amounts {
				out[k] = v
			}
		}
		out[name] = expr.Parse(head + "$" + changed)
	}
	if out == nil {
		return amounts
	}
	return out
}

// rewriteDef is the definition def has under word map m: every face's
// abilities, triggers, statics and replacements with their params rewritten,
// every modifiable keyword line rewritten, the face's type line under the
// type words and its amounts' counted text (ADR-0039).
func rewriteDef(def *compile.Card, m wordMap) *compile.Card {
	out := *def
	fn := func(s string) string { return rewriteText(s, m) }
	for i := range out.Faces {
		f := def.Faces[i]
		amounts := amountNames(f.Amounts)
		memo := map[*compile.Ability]*compile.Ability{}
		f.Abilities = rewriteAbilities(f.Abilities, fn, amounts, memo)
		f.Triggers = rewriteAbilities(f.Triggers, fn, amounts, memo)
		f.Statics = rewriteAbilities(f.Statics, fn, amounts, memo)
		f.Replacements = rewriteAbilities(f.Replacements, fn, amounts, memo)
		if f.Keywords != nil {
			kws := make([]string, len(f.Keywords))
			for j, kw := range f.Keywords {
				kws[j] = kw
				if keywordModifiable(kw) {
					kws[j] = rewriteText(kw, m)
				}
			}
			f.Keywords = kws
		}
		f.Type = rewriteType(f.Type, m)
		f.Amounts = rewriteAmounts(f.Amounts, fn)
		out.Faces[i] = f
	}
	return &out
}
