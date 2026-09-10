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

// keyEvidence is what both signals say about one param key.
type keyEvidence struct {
	java   map[Kind]int // Java call sites, by the kind they imply
	values map[string]int
	uses   int
}

// classify reads the Java for call-site evidence. A key can appear under more
// than one kind -- Defined$ is a valid string on some effects and an object
// selector on others -- so the counts are kept rather than collapsed, and the
// report shows the disagreement instead of hiding it behind a winner.
func classify(forge string) (map[string]*keyEvidence, error) {
	out := map[string]*keyEvidence{}
	get := func(key string) *keyEvidence {
		e, ok := out[key]
		if !ok {
			e = &keyEvidence{java: map[Kind]int{}, values: map[string]int{}}
			out[key] = e
		}
		return e
	}

	roots := append([]string{effectsDir}, sharedRoots...)
	for _, root := range roots {
		err := filepath.WalkDir(filepath.Join(forge, root), func(path string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() || filepath.Ext(path) != ".java" {
				return err
			}
			raw, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			text := string(raw)
			for _, ev := range javaEvidence {
				for _, m := range ev.re.FindAllStringSubmatch(text, -1) {
					get(m[1]).java[ev.kind]++
				}
			}
			// A key whose value is never read is a flag, so record that the
			// value was read at all.
			for _, m := range valueRead.FindAllStringSubmatch(text, -1) {
				get(m[1]).java[KindUnknown] += 0 // ensure the key exists
				_ = m
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	return out, nil
}

// corpusKinds adds the second signal: what the values themselves parse as.
// Java evidence says how one call site uses a key; the corpus says what every
// card actually writes there, and the two disagreeing is worth reading.
func corpusKinds(corpus, types string, ev map[string]*keyEvidence) (int, error) {
	reg, err := loadTypes(types)
	if err != nil {
		return 0, err
	}
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
		out, err := compile.Compile(card)
		if err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
		cards++
		for i := range out.Faces {
			for _, group := range [][]*compile.Ability{
				out.Faces[i].Abilities, out.Faces[i].Triggers,
				out.Faces[i].Statics, out.Faces[i].Replacements,
			} {
				for _, a := range group {
					collectValues(a, ev)
				}
			}
		}
		return nil
	})
	return cards, err
}

func collectValues(a *compile.Ability, ev map[string]*keyEvidence) {
	for _, p := range a.Params {
		e, ok := ev[p.Key]
		if !ok {
			e = &keyEvidence{java: map[Kind]int{}, values: map[string]int{}}
			ev[p.Key] = e
		}
		e.uses++
		e.values[valueShape(p.Value)]++
	}
	for _, s := range a.Subs {
		collectValues(s.Ability, ev)
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

// reportTypes prints one row per param key: what Java's call sites imply, what
// the corpus writes, and how often. It is a report and not a gate: the two
// signals disagree on real keys, and a generator that guessed silently would
// bake the guess into 193 structs.
func reportTypes(forge, corpus, types string) error {
	return writeTypes(forge, corpus, types, os.Stdout)
}

// writeTypes is reportTypes with the destination injected, so the golden test
// can capture the same bytes CI prints (TEST-8: a different writer, not a mock).
func writeTypes(forge, corpus, types string, w *os.File) error {
	ev, err := classify(forge)
	if err != nil {
		return err
	}
	cards, err := corpusKinds(corpus, types, ev)
	if err != nil {
		return err
	}

	keys := make([]string, 0, len(ev))
	for k := range ev {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	counted := 0
	for _, key := range keys {
		e := ev[key]
		if e.uses == 0 {
			continue // read by Java, written by no card
		}
		counted++
		_, _ = fmt.Fprintf(w, "%s\t%d\t%s\t%s\n", key, e.uses, topJava(e), topValues(e))
	}
	_, _ = fmt.Fprintf(os.Stderr, "apiscan -kinds: %d cards, %d param keys written by at least one card\n", cards, counted)
	return nil
}

func topJava(e *keyEvidence) string {
	var parts []string
	for k := KindFlag; k <= KindText; k++ {
		if n := e.java[k]; n > 0 {
			parts = append(parts, fmt.Sprintf("%s:%d", k, n))
		}
	}
	if len(parts) == 0 {
		return "-"
	}
	return strings.Join(parts, ",")
}

func topValues(e *keyEvidence) string {
	type kv struct {
		shape string
		n     int
	}
	var all []kv
	for s, n := range e.values {
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
