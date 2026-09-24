// Package cardtype holds the type line: supertypes, core types, and subtypes.
//
// A card script writes its type line as one space-separated string, and the
// engine has to answer "is this a creature", "is this an Equipment", "does this
// share a creature type with that" millions of times per batch. So the string
// is parsed once, at load, into the value type here (PORT-2), and the answers
// become bit tests.
//
// Ported from forge-core/src/main/java/forge/card/CardType.java.
// Deviations recorded in docs/crucible/porting/port-log/card-type.md.
package cardtype

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// Line is a parsed type line.
//
// The zero value is the empty type line, which is what a card gets while an
// effect is stripping its types. Supertypes and core types are sets and print
// in the fixed order the rules use; subtypes keep the order the card printed
// them in, because "Elf Warrior" and "Warrior Elf" are different type lines
// even though they mean the same thing.
type Line struct {
	supertypes uint8  // bit per Supertype
	coreTypes  uint16 // bit per CoreType
	subtypes   []string
}

// Separator is what a printed type line puts between core types and subtypes.
// Card scripts leave it out; Forge's own output and one card script include it.
const Separator = " - "

// Parse reads a type line: "Legendary Creature Elf Warrior".
//
// A "-" is an ordinary word and becomes a subtype, which is what Java does and
// therefore what the card database holds: `gandalf_shadows_foe` writes its type
// line in printed form, "Legendary Creature - Avatar Wizard", and Forge stores
// a literal "-" subtype for it. Parse(l.String()) is consequently not a fixed
// point for a line with subtypes, and matching the oracle is worth more than a
// property this parser was never required to have (PORT-7).
//
// Parse returns no error, because no type line can fail. Every word that is not
// a core type or a supertype becomes a subtype, including one the vocabulary has
// never heard of: Java's CardType.parse adds each word with add(), which never
// runs sanisfySubtypes, so Forge's own card database holds Contraption and
// Killbot exactly as the script wrote them.
//
// Dropping a subtype is something Java does only on the type-changing path
// (addAll, removeAll, getTypeWithChanges), which belongs to the layer system at
// M4. Doing it here would make 77 cards differ from the oracle.
//
// [UnknownTypes] reports the words the vocabulary does not contain, which is
// what the P2 vocabulary gate consumes.
//
// A nil registry is a programming error, not card data, so it panics (GO-7).
func Parse(reg *Registry, text string) Line {
	if reg == nil {
		panic("cardtype.Parse: nil registry")
	}

	var out Line
	var subtypes []string
	for _, word := range splitTypes(reg, strings.TrimSpace(text)) {
		if core, ok := CoreTypeFromName(word); ok {
			out.coreTypes |= 1 << uint(core)
			continue
		}
		if super, ok := SupertypeFromName(word); ok {
			out.supertypes |= 1 << uint(super)
			continue
		}
		subtypes = append(subtypes, word)
	}

	out.subtypes = subtypes
	return out
}

// UnknownTypes returns the words in a type line that are in no category of the
// vocabulary, in the order they appear.
//
// These are the words Forge silently discards: joke-set types such as Killbot
// and Clamfolk, and types newer than the vocabulary file, such as Omenpath.
// Reporting them is what turns a silent data gap into a reviewable list, and is
// the hook the P2 vocabulary gate will use.
func UnknownTypes(reg *Registry, text string) []string {
	if reg == nil {
		panic("cardtype.UnknownTypes: nil registry")
	}

	var out []string
	for _, word := range splitTypes(reg, strings.TrimSpace(text)) {
		if _, ok := CoreTypeFromName(word); ok {
			continue
		}
		if _, ok := SupertypeFromName(word); ok {
			continue
		}
		if !reg.Known(word) {
			out = append(out, word)
		}
	}
	return out
}

// splitTypes breaks a type line into words, keeping multiword subtypes such as
// "Time Lord" and "Bolas's Meditation Realm" whole.
func splitTypes(reg *Registry, text string) []string {
	var out []string
	for i := 0; i < len(text); {
		if text[i] == ' ' {
			i++
			continue
		}
		if multi, ok := reg.multiwordPrefix(text[i:]); ok {
			out = append(out, multi)
			i += len(multi)
			continue
		}
		j := strings.IndexByte(text[i:], ' ')
		if j < 0 {
			out = append(out, text[i:])
			break
		}
		out = append(out, text[i:i+j])
		i += j
	}
	return out
}

// Known reports whether a word appears in any subtype category.
//
// Exported for the valid-property gate: `CardProperty` accepts a bare subtype
// as a property, so deciding whether a property is implemented means asking
// the subtype vocabulary the same question Java asks it.
func (r *Registry) Known(name string) bool {
	for c := Category(0); int(c) < numCategories; c++ {
		if r.members[c][name] {
			return true
		}
	}
	return false
}

// Has reports whether the line carries a core type.
func (l Line) Has(t CoreType) bool { return l.coreTypes&(1<<uint(t)) != 0 }

// HasSupertype reports whether the line carries a supertype.
func (l Line) HasSupertype(t Supertype) bool { return l.supertypes&(1<<uint(t)) != 0 }

// HasSubtype reports whether the line carries a subtype, compared exactly as
// printed.
func (l Line) HasSubtype(name string) bool {
	for _, s := range l.subtypes {
		if s == name {
			return true
		}
	}
	return false
}

// HasStringType reports whether the line carries the named core type,
// supertype or subtype -- ported from CardType.hasStringType, which valid
// strings and card scripts alike use to ask "is this a Creature" or "is
// this an Elf" by the word as written, without the caller knowing which of
// the three the word names.
//
// A subtype is checked first, exactly as written (HasSubtype's own
// case-sensitive match): a script never has reason to write a subtype in
// anything but its printed case. Only if that fails does the name get
// Java's one-character capitalization (StringUtils.capitalize: the first
// rune uppercased, everything after left alone) before it is tried as a
// core type, then a supertype -- catching a script that writes "creature"
// lowercase, which a subtype never would.
func (l Line) HasStringType(name string) bool {
	if name == "" {
		return false
	}
	if l.HasSubtype(name) {
		return true
	}
	name = capitalizeFirst(name)
	if t, ok := CoreTypeFromName(name); ok {
		return l.Has(t)
	}
	if t, ok := SupertypeFromName(name); ok {
		return l.HasSupertype(t)
	}
	return false
}

func capitalizeFirst(s string) string {
	r, size := utf8.DecodeRuneInString(s)
	if r == utf8.RuneError || unicode.IsUpper(r) {
		return s
	}
	return string(unicode.ToUpper(r)) + s[size:]
}

// IsPermanent reports whether a card with this type line stays on the
// battlefield. A card with several core types is a permanent if any of them is.
func (l Line) IsPermanent() bool {
	for t := CoreType(0); int(t) < numCoreTypes; t++ {
		if l.Has(t) && t.IsPermanent() {
			return true
		}
	}
	return false
}

// IsEmpty reports whether the line carries no types at all.
func (l Line) IsEmpty() bool {
	return l.coreTypes == 0 && l.supertypes == 0 && len(l.subtypes) == 0
}

// CoreTypes returns the core types in print order.
func (l Line) CoreTypes() []CoreType {
	var out []CoreType
	for t := CoreType(0); int(t) < numCoreTypes; t++ {
		if l.Has(t) {
			out = append(out, t)
		}
	}
	return out
}

// Supertypes returns the supertypes in print order.
func (l Line) Supertypes() []Supertype {
	var out []Supertype
	for t := Supertype(0); int(t) < numSupertypes; t++ {
		if l.HasSupertype(t) {
			out = append(out, t)
		}
	}
	return out
}

// Subtypes returns the subtypes in printed order. The slice aliases the line's
// storage; callers must not modify it.
func (l Line) Subtypes() []string { return l.subtypes }

// CreatureTypes returns the subtypes that are creature types, which is only
// meaningful on a creature or a Kindred card.
func (l Line) CreatureTypes(reg *Registry) []string {
	if !l.Has(Creature) && !l.Has(Kindred) {
		return nil
	}
	var out []string
	for _, s := range l.subtypes {
		if reg.IsCreatureType(s) {
			out = append(out, s)
		}
	}
	return out
}

// Equal reports whether two type lines are the same, subtype order included.
// Order is part of the identity here because it is part of what the card
// printed and what the oracle dump compares.
func (l Line) Equal(other Line) bool {
	if l.coreTypes != other.coreTypes || l.supertypes != other.supertypes {
		return false
	}
	if len(l.subtypes) != len(other.subtypes) {
		return false
	}
	for i, s := range l.subtypes {
		if s != other.subtypes[i] {
			return false
		}
	}
	return true
}

// ParseToken classifies one already-separated type word into a one-token
// Line -- Parse's own per-word classification (core type, then supertype,
// then subtype fallthrough), pulled out for a caller that already has a
// single word in hand and needs no multiword lookahead, so no Registry.
//
// CR 613.4's own AddType$/RemoveType$ (a Mode$ Continuous static ability,
// StaticAbilityContinuous.java) is exactly that caller: its own value is
// already split on " & " into individual type names by the time it reaches
// here, and the engine that folds those into a card's current type line has
// no *Registry to give Parse (game-state.md's "Not ported yet" -- this port
// injects no carddb.DB into the engine yet). A word Parse would have needed
// Registry to tell apart from a multiword subtype's own first word (rare in
// this exact position -- AddType$/RemoveType$ values are single type names,
// not printed type lines) is treated as one whole subtype, the same
// fallthrough Parse itself would give an unrecognized word.
func ParseToken(word string) Line {
	if core, ok := CoreTypeFromName(word); ok {
		return Line{coreTypes: 1 << uint(core)}
	}
	if super, ok := SupertypeFromName(word); ok {
		return Line{supertypes: 1 << uint(super)}
	}
	return Line{subtypes: []string{word}}
}

// Union returns a Line carrying every supertype, core type and subtype in l
// or other -- CR 613.4's own "add" direction. Subtypes are deduplicated (a
// type already on l is not appended again) but otherwise order-stable: l's
// own subtypes first, printed order preserved, then any of other's l did
// not already have.
func (l Line) Union(other Line) Line {
	out := Line{supertypes: l.supertypes | other.supertypes, coreTypes: l.coreTypes | other.coreTypes}
	out.subtypes = append(out.subtypes, l.subtypes...)
	for _, s := range other.subtypes {
		if !l.HasSubtype(s) {
			out.subtypes = append(out.subtypes, s)
		}
	}
	return out
}

// Without returns l with every supertype, core type and subtype in other
// cleared -- CR 613.4's own "remove" direction. A supertype/core type/
// subtype other does not carry is left exactly as l had it.
func (l Line) Without(other Line) Line {
	out := Line{supertypes: l.supertypes &^ other.supertypes, coreTypes: l.coreTypes &^ other.coreTypes}
	for _, s := range l.subtypes {
		if !other.HasSubtype(s) {
			out.subtypes = append(out.subtypes, s)
		}
	}
	return out
}

// String prints the line the way a card does: supertypes, then core types, then
// the separator and the subtypes.
func (l Line) String() string {
	var b strings.Builder
	for _, t := range l.Supertypes() {
		if b.Len() > 0 {
			b.WriteByte(' ')
		}
		b.WriteString(t.String())
	}
	for _, t := range l.CoreTypes() {
		if b.Len() > 0 {
			b.WriteByte(' ')
		}
		b.WriteString(t.String())
	}
	if len(l.subtypes) > 0 {
		b.WriteString(Separator)
		b.WriteString(strings.Join(l.subtypes, " "))
	}
	return b.String()
}

// WithoutCardTypes clears every core type except Instant and Sorcery --
// CardChangedType.applyChanges' RemoveCardTypes, which keeps them per CR
// 205.1a ("an object with either the instant or sorcery card type retains
// that type").
func (l Line) WithoutCardTypes() Line {
	keep := uint16(1)<<uint(Instant) | uint16(1)<<uint(Sorcery)
	out := l
	out.coreTypes &= keep
	out.subtypes = append([]string(nil), l.subtypes...)
	return out
}

// WithoutSupertypes clears every supertype (RemoveSuperTypes).
func (l Line) WithoutSupertypes() Line {
	out := l
	out.supertypes = 0
	out.subtypes = append([]string(nil), l.subtypes...)
	return out
}

// WithoutSubtypes clears every subtype (RemoveSubTypes).
func (l Line) WithoutSubtypes() Line {
	out := l
	out.subtypes = nil
	return out
}

// WithoutSubtypesWhere clears every subtype drop reports true for --
// RemoveCreatureTypes/RemoveLandTypes/RemoveArtifactTypes/
// RemoveEnchantmentTypes, each a Registry membership test.
func (l Line) WithoutSubtypesWhere(drop func(string) bool) Line {
	out := l
	out.subtypes = nil
	for _, s := range l.subtypes {
		if !drop(s) {
			out.subtypes = append(out.subtypes, s)
		}
	}
	return out
}
