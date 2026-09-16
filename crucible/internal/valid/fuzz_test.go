package valid_test

import (
	"reflect"
	"testing"

	"github.com/jczastkiewicz/crucible/internal/valid"
)

// FuzzValidString checks the same TEST-10 parser properties FuzzManaCost
// (internal/mana) checks for mana.Parse -- never panic, and parse-then-print
// is a fixed point -- for valid.Parse. TEST-2 names "valid strings" directly
// as an example of the "pure function, huge input space" category that wants
// a fuzz target alongside its table test, and this package never had one.
//
// valid.Parse itself never fails (its own doc comment: "Java has no parse
// step to fail in" -- an unrecognised base or property just matches
// nothing), so there is no error case to check here, only that nothing
// panics and that Parse(spec.String()) reproduces spec exactly -- the same
// "print of a parse is stable under a second parse" property FuzzManaCost
// checks, not that the printed form matches the original fuzz input (which
// the corpus test's own "padded" exception already shows does not always
// hold for input a script would never actually write).
func FuzzValidString(f *testing.F) {
	seeds := []string{
		"", "Card", "Creature", "Creature.YouCtrl", "Creature.powerGE1",
		"Instant.YouCtrl+nonToken,Sorcery,Card.!token", "!Ongoing", "Self",
		"EnchantedBy$CardCounters.P1P1", "Card.EnchantedBy$CardCounters.P1P1",
		"basePowerEQ2", "baseToughnessEQ1", "cmcEQChosen", "cmcEQX", "cmcGE7",
		"Permanent.SharesColorWith Valid Creature.YouCtrl",
		"Player, Planeswalker", ",", ".", "..", "+", "!", "!!", "$", "Card.",
		"Card..", "Card.+", "toughnessLE", "totalPT_GE1", "numColorsGE1",
		"numTypesGE1", " Land", "Land ", "a,b,c", "a.b.c", "a+b+c",
	}
	for _, s := range seeds {
		f.Add(s)
	}

	f.Fuzz(func(t *testing.T, in string) {
		spec := valid.Parse(in)

		printed := spec.String()
		again := valid.Parse(printed)
		if !reflect.DeepEqual(again, spec) {
			t.Fatalf("Parse(%q) = %+v, printed %q, reparsed as %+v", in, spec, printed, again)
		}
		if again.String() != printed {
			t.Fatalf("Parse(%q) printed %q, reparse printed %q", in, printed, again.String())
		}
	})
}
