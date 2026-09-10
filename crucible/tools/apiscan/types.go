// Param types, recovered from two independent signals.

package main

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/jczastkiewicz/crucible/internal/carddb"
	"github.com/jczastkiewicz/crucible/internal/carddb/compile"
	"github.com/jczastkiewicz/crucible/internal/expr"
)

// Kind is what a param's value is, which decides the field type the generated
// struct gets: expr.Amount, valid.Spec, a zone, a plain string (ADR-0007).
type Kind uint8

// The kinds, in the order the report lists them.
const (
	KindUnknown Kind = iota
	KindFlag         // presence only; nothing reads the value
	KindAmount       // a count expression, through AbilityUtils.calculateAmount
	KindValid        // a valid string, through getValidCards / isValid / matchesValid
	KindDefined      // an object selector, through getDefinedCards and friends
	KindZone         // ZoneType.smartValueOf / listValueOf
	KindCounter      // CounterType.getType
	KindInt          // Integer.parseInt
	KindSVarRef      // names an SVar whose body is read
	KindText         // prose: descriptions, prompts, stack text
)

var kindNames = [...]string{
	KindUnknown: "unknown", KindFlag: "flag", KindAmount: "amount", KindValid: "valid",
	KindDefined: "defined", KindZone: "zone", KindCounter: "counter", KindInt: "int",
	KindSVarRef: "svarref", KindText: "text",
}

// String returns the kind's name.
func (k Kind) String() string { return kindNames[k] }

// javaEvidence maps a Java call to the kind of the param that flows into it.
// The window is bounded rather than greedy: a param twenty characters into a
// call is an argument to it, a param two hundred characters away is a
// different call on the same line.
var javaEvidence = []struct {
	kind Kind
	re   *regexp.Regexp
}{
	{KindAmount, regexp.MustCompile(`calculateAmount\([^;]{0,80}?getParam(?:OrDefault)?\("([A-Za-z]+)"`)},
	{KindValid, regexp.MustCompile(`(?:getValidCards|getValidPlayers|isValid|matchesValid)\([^;]{0,80}?getParam(?:OrDefault)?\("([A-Za-z]+)"`)},
	{KindDefined, regexp.MustCompile(`getDefined[A-Za-z]*\([^;]{0,80}?getParam(?:OrDefault)?\("([A-Za-z]+)"`)},
	{KindZone, regexp.MustCompile(`ZoneType\.(?:smartValueOf|listValueOf)\(\s*(?:sa|host)?\.?getParam(?:OrDefault)?\("([A-Za-z]+)"`)},
	{KindCounter, regexp.MustCompile(`CounterType\.getType\(\s*(?:sa|host)?\.?getParam(?:OrDefault)?\("([A-Za-z]+)"`)},
	{KindInt, regexp.MustCompile(`Integer\.parseInt\(\s*(?:sa|host)?\.?getParam(?:OrDefault)?\("([A-Za-z]+)"`)},
	{KindSVarRef, regexp.MustCompile(`getSVar\(\s*(?:sa|host)?\.?getParam(?:OrDefault)?\("([A-Za-z]+)"`)},
	{KindText, regexp.MustCompile(`(?:append|setStackDescription)\(\s*(?:sa|host)?\.?getParam(?:OrDefault)?\("([A-Za-z]+)"`)},
}

// valueRead matches any read of a param's value, as opposed to a test for its
// presence. A key never read this way is a flag.
var valueRead = regexp.MustCompile(`getParam(?:OrDefault)?\("([A-Za-z]+)"`)

// pair is one (API, param key). The type belongs to the pair, not the key.
type pair struct{ api, key string }

// written is what the corpus writes for one pair.
type written struct {
	values map[string]int
	uses   int
}

// classify reads the Java for call-site evidence, per API.
//
// The measurement that motivated this: AttachedTo is a valid string on some
// effects and an object selector on others, and Choices likewise. A per-key
// type would be wrong for one of them, so evidence found inside an effect
// class belongs to that API, and evidence anywhere else is the shared
// fallback a key falls back to when its own effect says nothing.
func classify(forge string, apis []api) (perAPI map[string]map[string]map[Kind]int, shared map[string]map[Kind]int, err error) {
	perAPI = map[string]map[string]map[Kind]int{}
	shared = map[string]map[Kind]int{}

	scan := func(text string, into map[string]map[Kind]int) {
		for _, ev := range javaEvidence {
			for _, m := range ev.re.FindAllStringSubmatch(text, -1) {
				if into[m[1]] == nil {
					into[m[1]] = map[Kind]int{}
				}
				into[m[1]][ev.kind]++
			}
		}
	}

	// Per API: the effect class and every ancestor of it inside the effects
	// directory, the same chain readEffectParams follows.
	effects := filepath.Join(forge, effectsDir)
	for _, a := range apis {
		into := map[string]map[Kind]int{}
		class := a.class
		for depth := 0; depth < 8; depth++ {
			raw, readErr := os.ReadFile(filepath.Join(effects, class+".java"))
			if readErr != nil {
				break
			}
			scan(string(raw), into)
			m := effectSuper.FindStringSubmatch(string(raw))
			if m == nil {
				break
			}
			class = m[1]
		}
		perAPI[a.name] = into
	}

	// Shared: everything outside the effects directory.
	for _, root := range sharedRoots {
		walkErr := filepath.WalkDir(filepath.Join(forge, root), func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				if filepath.Clean(path) == filepath.Clean(effects) {
					return filepath.SkipDir
				}
				return nil
			}
			if filepath.Ext(path) != ".java" {
				return nil
			}
			raw, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			scan(string(raw), shared)
			return nil
		})
		if walkErr != nil {
			return nil, nil, walkErr
		}
	}
	return perAPI, shared, nil
}

// corpusKinds adds the second signal: what the values themselves parse as,
// per API. Java evidence says how one call site uses a key; the corpus says
// what every card actually writes there, and the two disagreeing is worth
// reading rather than averaging away.
func corpusKinds(corpus, types string) (map[pair]*written, int, error) {
	reg, err := loadTypes(types)
	if err != nil {
		return nil, 0, err
	}
	out := map[pair]*written{}
	cards := 0
	err = filepath.WalkDir(corpus, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || filepath.Ext(path) != ".txt" {
			return err
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		card, err := carddb.ParseScript(reg, strings.TrimSuffix(filepath.Base(path), ".txt"), raw)
		if err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
		out2, err := compile.Compile(card)
		if err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
		cards++
		for i := range out2.Faces {
			for _, group := range [][]*compile.Ability{
				out2.Faces[i].Abilities, out2.Faces[i].Triggers,
				out2.Faces[i].Statics, out2.Faces[i].Replacements,
			} {
				for _, a := range group {
					collectValues(a, out)
				}
			}
		}
		return nil
	})
	return out, cards, err
}

func collectValues(a *compile.Ability, out map[pair]*written) {
	for _, p := range a.Params {
		k := pair{api: a.Name, key: p.Key}
		w, ok := out[k]
		if !ok {
			w = &written{values: map[string]int{}}
			out[k] = w
		}
		w.uses++
		w.values[valueShape(p.Value)]++
	}
	for _, s := range a.Subs {
		collectValues(s.Ability, out)
	}
}

// valueShape is what a written value looks like, and it is deliberately a
// weak signal.
//
// expr.Parse and valid.Parse are total -- every string is an amount of some
// sort and a valid string of some sort -- so acceptance says nothing. What the
// corpus can settle is narrow: a key whose every value is True or False is a
// flag, a key whose values carry an expression head is an amount, and a value
// with spaces in it is prose rather than a token. Telling a zone from a valid
// string from an SVar name needs the Java call site; both signals are reported
// because neither is sufficient alone.
func valueShape(v string) string {
	switch {
	case strings.EqualFold(v, "True"), strings.EqualFold(v, "False"):
		return "bool"
	case isInt(v):
		return "int"
	}
	if expr.Parse(v).Kind == expr.Expression {
		return "amount"
	}
	if strings.ContainsAny(v, " \t") {
		return "prose"
	}
	return "token"
}

func isInt(v string) bool {
	if v == "" {
		return false
	}
	_, err := strconv.Atoi(strings.TrimPrefix(strings.TrimPrefix(v, "+"), "-"))
	return err == nil
}

// reportTypes prints one row per (API, key): what that API's own Java implies,
// what the shared code implies when the API says nothing, and what the corpus
// writes. It is a report, not a gate: the two signals disagree on real pairs,
// and a generator that picked a winner silently would bake the guess into 193
// structs.
func reportTypes(forge, corpus, types string) error {
	return writeTypes(forge, corpus, types, os.Stdout)
}

// writeTypes is reportTypes with the destination injected, so the golden test
// captures the same bytes CI prints (TEST-8: a different writer, not a mock).
func writeTypes(forge, corpus, types string, w *os.File) error {
	apis, _, err := readVocabulary(forge, true)
	if err != nil {
		return err
	}
	perAPI, shared, err := classify(forge, apis)
	if err != nil {
		return err
	}
	corpusUse, cards, err := corpusKinds(corpus, types)
	if err != nil {
		return err
	}

	pairs := make([]pair, 0, len(corpusUse))
	for p := range corpusUse {
		pairs = append(pairs, p)
	}
	sort.Slice(pairs, func(i, j int) bool {
		if pairs[i].api != pairs[j].api {
			return pairs[i].api < pairs[j].api
		}
		return pairs[i].key < pairs[j].key
	})

	for _, p := range pairs {
		own := "-"
		if m := perAPI[p.api]; m != nil {
			own = fmtKinds(m[p.key])
		}
		_, _ = fmt.Fprintf(w, "%s\t%s\t%d\t%s\t%s\t%s\n",
			p.api, p.key, corpusUse[p].uses, own, fmtKinds(shared[p.key]), fmtValues(corpusUse[p]))
	}
	_, _ = fmt.Fprintf(os.Stderr, "apiscan -kinds: %d cards, %d (API, key) pairs\n", cards, len(pairs))
	return nil
}

func fmtKinds(m map[Kind]int) string {
	if len(m) == 0 {
		return "-"
	}
	var parts []string
	for k := KindFlag; k <= KindText; k++ {
		if n := m[k]; n > 0 {
			parts = append(parts, fmt.Sprintf("%s:%d", k, n))
		}
	}
	if len(parts) == 0 {
		return "-"
	}
	return strings.Join(parts, ",")
}

func fmtValues(w *written) string {
	type kv struct {
		shape string
		n     int
	}
	var all []kv
	for s, n := range w.values {
		all = append(all, kv{s, n})
	}
	sort.Slice(all, func(i, j int) bool {
		if all[i].n != all[j].n {
			return all[i].n > all[j].n
		}
		return all[i].shape < all[j].shape
	})
	var parts []string
	for _, x := range all {
		parts = append(parts, fmt.Sprintf("%s:%d", x.shape, x.n))
	}
	return strings.Join(parts, ",")
}
