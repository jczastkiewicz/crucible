// BoosterGenerator.makeSheet: a sheet key such as "Common DOM",
// `RareMythic:fromSheet("DOM cards")` or `promo("Name|SET")` turned into a
// print sheet over the card pool (BoosterGenerator.java:665-760, :765-840).

package boosters

import (
	"fmt"
	"strings"
)

// predicate is a filter over prints.
type predicate func(*Print) bool

func isRarity(r Rarity) predicate { return func(p *Print) bool { return p.Rarity == r } }

// splitParen is TextUtil.splitWithParenthesis(input, delimiter, max, '(', ')',
// skipEmpty): split on the delimiter outside parentheses, empty parts dropped,
// the remainder after max-1 splits kept whole.
func splitParen(input string, delim byte, limit int) []string {
	var out []string
	nPar, start, idx := 0, 0, 1
	for i := 0; i < len(input); i++ {
		c := input[i]
		if c == ')' && nPar > 0 {
			nPar--
		} else if c == '(' {
			nPar++
		}
		if c == delim && nPar == 0 && idx < limit {
			if i > start {
				out = append(out, input[start:i])
				idx++
			}
			start = i + 1
		}
	}
	if len(input) > start {
		out = append(out, input[start:])
	}
	return out
}

// splitQuoted is TextUtil.splitWithParenthesis(input, ',', '"', '"'): the
// quotes are the parentheses and are stripped from each part.
func splitQuoted(input string) []string {
	var out []string
	inQ, start := false, 0
	flush := func(end int) {
		if end > start {
			out = append(out, strings.Trim(input[start:end], `"`))
		}
	}
	for i := 0; i < len(input); i++ {
		c := input[i]
		if c == '"' {
			inQ = !inQ
		}
		if c == ',' && !inQ {
			flush(i)
			start = i + 1
		}
	}
	flush(len(input))
	return out
}

func stripChars(s, chars string) string { return strings.Trim(s, chars) }

// sheet is the generator's cached makeSheet by key (BoosterGenerator.getPrintSheet).
func (g *Generator) sheet(key string) (*Sheet, error) {
	if s, ok := g.cache[fold(key)]; ok {
		return s, nil
	}
	s, err := g.makeSheet(key)
	if err != nil {
		return nil, err
	}
	g.cache[fold(key)] = s
	return s, nil
}

func (g *Generator) makeSheet(key string) (*Sheet, error) {
	t := g.t
	ps := newSheet(key)
	sKey := splitParen(key, ' ', 2)
	if len(sKey) == 0 {
		return nil, fmt.Errorf("boosters: empty sheet key")
	}
	setPred := predicate(func(*Print) bool { return true })
	if len(sKey) > 1 {
		sets := map[string]bool{}
		for _, s := range strings.Split(sKey[1], " ") {
			sets[fold(s)] = true
		}
		setPred = func(p *Print) bool {
			if !sets[fold(p.Edition)] {
				return false
			}
			e := t.Pool.edition(p.Edition)
			if e == nil {
				return true
			}
			for _, c := range e.cardSections["conjured"] {
				if c.name == p.Name {
					return false
				}
			}
			return true
		}
	}

	operators := splitParen(sKey[0], ':', 1<<30)
	extra, operators, err := g.buildExtraPredicate(operators)
	if err != nil {
		return nil, err
	}

	src := t.Pool.all
	var kept []string
	for _, code := range operators {
		lower := strings.ToLower(code)
		switch {
		case strings.HasPrefix(lower, "fromsheet") || strings.HasPrefix(lower, "wholesheet"):
			if len(code) < 10 {
				return nil, fmt.Errorf("boosters: sheet %q: operator %q", key, code)
			}
			name := stripChars(code[10:], `()" `)
			from, err := t.staticSheet(name)
			if err != nil {
				return nil, err
			}
			if from == nil {
				return nil, fmt.Errorf("boosters: PrintSheet Error: %s didn't find %s from %s", key, name, code)
			}
			src = from.flat()
			setPred = func(*Print) bool { return true }
		case strings.HasPrefix(code, "promo") || strings.HasPrefix(code, "name"):
			list := stripChars(code[5:], "() ")
			var srcList []*Print
			for _, name := range splitQuoted(list) {
				pr, err := t.Pool.getCard(name)
				if err != nil {
					return nil, err
				}
				srcList = append(srcList, pr)
			}
			src = srcList
			setPred = func(*Print) bool { return true }
		default:
			kept = append(kept, code)
		}
	}
	operators = kept

	main := ""
	if len(operators) > 0 {
		main = strings.TrimSpace(operators[0])
	}
	filter := func(weight int, preds ...predicate) error {
		for _, p := range src {
			ok := setPred(p) && extra(p)
			for _, f := range preds {
				ok = ok && f(p)
			}
			if ok {
				if err := ps.add(p, weight); err != nil {
					return err
				}
			}
		}
		return nil
	}
	switch {
	case main == "" || strings.EqualFold(main, "Any"):
		err = filter(1)
	case strings.EqualFold(main, "UncommonRare"):
		if err = filter(1, isRarity(Rare)); err == nil {
			err = filter(3, isRarity(Uncommon))
		}
	case strings.EqualFold(main, "RareMythic"):
		if err = filter(1, isRarity(MythicRare)); err == nil {
			err = filter(2, isRarity(Rare))
		}
	case strings.EqualFold(main, "UncommonRareMythic"):
		if err = filter(1, isRarity(MythicRare)); err == nil {
			if err = filter(2, isRarity(Rare)); err == nil {
				err = filter(4, isRarity(Uncommon))
			}
		}
	default:
		return nil, fmt.Errorf("boosters: Booster generator: operator could not be parsed - %s", main)
	}
	if err != nil {
		return nil, err
	}
	return ps, nil
}

// buildExtraPredicate is BoosterGenerator.buildExtraPredicate: the operators
// that are plain filters become one predicate and leave the list.
func (g *Generator) buildExtraPredicate(operators []string) (predicate, []string, error) {
	var conds []predicate
	var rest []string
	for _, op := range operators {
		if op == "" {
			continue
		}
		operator := op
		operator = strings.TrimSuffix(operator, "s")
		invert := operator[0] == '!'
		if invert {
			operator = operator[1:]
		}
		var toAdd predicate
		switch {
		case strings.EqualFold(operator, "dfc"):
			toAdd = func(p *Print) bool { return p.dfc }
		case strings.EqualFold(operator, "Land"):
			toAdd = func(p *Print) bool { return p.land }
		case strings.EqualFold(operator, "BasicLand"):
			toAdd = isRarity(BasicLand)
		case strings.EqualFold(operator, "TimeShifted"), strings.EqualFold(operator, "Special"):
			toAdd = isRarity(Special)
		case strings.EqualFold(operator, "Mythic"):
			toAdd = isRarity(MythicRare)
		case strings.EqualFold(operator, "Rare"):
			toAdd = isRarity(Rare)
		case strings.EqualFold(operator, "Uncommon"):
			toAdd = isRarity(Uncommon)
		case strings.EqualFold(operator, "Common"):
			toAdd = isRarity(Common)
		case strings.HasPrefix(operator, "name("):
			names := map[string]bool{}
			for _, n := range splitQuoted(stripChars(operator[4:], "() ")) {
				names[n] = true
			}
			toAdd = func(p *Print) bool { return names[p.Name] }
		case strings.HasPrefix(operator, "color("), strings.HasPrefix(operator, "fromSets("):
			return nil, nil, fmt.Errorf("%w: operator %q", ErrNotModelled, op)
		case strings.HasPrefix(operator, "fromSheet(") && invert:
			sheetName := stripChars(operator[9:], `()" `)
			from, err := g.t.staticSheet(sheetName)
			if err != nil {
				return nil, nil, err
			}
			if from == nil {
				return nil, nil, fmt.Errorf("boosters: sheet %q not found", sheetName)
			}
			in := map[*Print]bool{}
			for _, p := range from.prints {
				in[p] = true
			}
			toAdd = func(p *Print) bool { return in[p] }
		}
		if toAdd == nil {
			rest = append(rest, op)
			continue
		}
		if invert {
			f := toAdd
			toAdd = func(p *Print) bool { return !f(p) }
		}
		conds = append(conds, toAdd)
	}
	if len(conds) == 0 {
		return func(*Print) bool { return true }, rest, nil
	}
	return func(p *Print) bool {
		for _, c := range conds {
			if !c(p) {
				return false
			}
		}
		return true
	}, rest, nil
}
