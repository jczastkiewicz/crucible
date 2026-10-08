package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
)

// TestWordRewriteMatchesJavasRegex proves AbilityUtils.getReplacedText's
// pattern, "(?<!named.{0,100})\b(non)?<word>" with no right-hand boundary,
// through a ChangeText on a card whose ability params carry each shape:
// a "non" prefix kept, a word inside a longer word (left boundary only),
// a lower-case color word, Any standing for every color, text after "named"
// left alone, params that name an SVar skipped, and LockInText$ keeping a
// whole trait.
func TestWordRewriteMatchesJavasRegex(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name       string
		change     string
		param      string
		value      string
		want       string
		wantParam2 string
	}{
		{"non prefix kept", "ChangeColorWord$ Red Blue", "ValidTgts", "Creature.nonRed", "Creature.nonBlue", ""},
		{"left boundary only", "ChangeColorWord$ Red Blue", "ValidTgts", "Creature.Reduce", "Creature.Blueuce", ""},
		{"no left boundary inside a word", "ChangeColorWord$ Red Blue", "ValidTgts", "Creature.Hundred", "Creature.Hundred", ""},
		{"lower case follows", "ChangeColorWord$ Red Blue", "Description", "red", "red", ""},
		{"any color", "ChangeColorWord$ Any Blue", "ValidTgts", "Creature.White+Red", "Creature.Blue+Blue", ""},
		{"any skips the destination", "ChangeColorWord$ Any Blue", "ValidTgts", "Creature.Blue+Green", "Creature.Blue+Blue", ""},
		{"after named is protected", "ChangeColorWord$ Red Blue", "ValidTgts", "Creature.namedFoo+Red", "Creature.namedFoo+Red", ""},
		{"before named is not", "ChangeColorWord$ Red Blue", "ValidTgts", "Creature.Red+namedFoo", "Creature.Blue+namedFoo", ""},
		{"type words are case sensitive", "ChangeTypeWord$ Elf Giant", "ValidTgts", "Creature.Elf+elf", "Creature.Giant+elf", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			g, p := textGame(t)
			def := scriptDef(t, "Subject", "Creature Elf", "A:AB$ Pump | Cost$ T | "+tc.param+"$ "+tc.value+" | NumAtt$ 1")
			subject := g.NewCard(def, p, engine.Battlefield)
			changeText(t, g, p, engine.NewScriptedController(),
				"DB$ ChangeText | Defined$ Targeted | "+tc.change+" | Duration$ Permanent", subject)
			got, _ := g.Card(subject).Def.Faces[0].Abilities[0].Param(tc.param)
			if got != tc.want {
				t.Errorf("%s = %q, want %q", tc.param, got, tc.want)
			}
		})
	}
}

// TestWordRewriteSkipsSVarNamesAndLockedTraits proves a param value that is an
// SVar's name is never rewritten (CardTraitBase.java:703-705) and a trait with
// LockInText$ keeps its text whole.
func TestWordRewriteSkipsSVarNamesAndLockedTraits(t *testing.T) {
	t.Parallel()

	g, p := textGame(t)
	def := scriptDef(t, "Subject", "Creature Elf",
		"A:AB$ Pump | Cost$ T | ValidTgts$ Creature | NumAtt$ RedCount | SubAbility$ DBRed",
		"A:AB$ Pump | Cost$ T | ValidTgts$ Creature.Red | NumAtt$ 1 | LockInText$ True",
		"SVar:RedCount:Count$Valid Creature.Red",
		"SVar:DBRed:DB$ Pump | Defined$ Self | NumAtt$ 1")
	subject := g.NewCard(def, p, engine.Battlefield)
	changeText(t, g, p, engine.NewScriptedController(),
		"DB$ ChangeText | Defined$ Targeted | ChangeColorWord$ Red Blue | Duration$ Permanent", subject)

	first := g.Card(subject).Def.Faces[0].Abilities[0]
	if got, _ := first.Param("NumAtt"); got != "RedCount" {
		t.Errorf("NumAtt = %q, want the SVar name RedCount left alone", got)
	}
	if got, _ := first.Param("SubAbility"); got != "DBRed" {
		t.Errorf("SubAbility = %q, want DBRed left alone", got)
	}
	if len(first.Subs) != 1 || first.Subs[0].Ability == nil {
		t.Fatalf("Subs = %v, want the compiled sub-ability kept", first.Subs)
	}
	if got, _ := g.Card(subject).Def.Faces[0].Abilities[1].Param("ValidTgts"); got != "Creature.Red" {
		t.Errorf("locked ValidTgts = %q, want Creature.Red", got)
	}
}
