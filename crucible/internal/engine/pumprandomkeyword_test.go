package engine_test

import (
	"slices"
	"testing"

	"github.com/jczastkiewicz/crucible/pkg/javarand"
)

// TestPumpRandomKeywordGrantsTheDrawnKeywords proves RandomKeyword$ with
// RandomKWNum$ (PumpEffect.java:380-398): two keywords come off the KW$ list
// by Aggregates.random's draws, each removing the first equal value from
// what is left.
func TestPumpRandomKeywordGrantsTheDrawnKeywords(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGame(t)
	def := etbPumpTriggerDefParams(t, "Test Random Keyword",
		`Defined$ Self | KW$ Flying & Haste & Trample | RandomKeyword$ True | RandomKWNum$ 2 | Duration$ Permanent`, nil)
	creature, err := castETBPump(t, g, p, def)
	if err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}

	r := javarand.New(1)
	total := []string{"Flying", "Haste", "Trample"}
	var want []string
	for range 2 {
		pick := total[r.Int32n(int32(len(total)))]
		want = append(want, pick)
		total = slices.Delete(total, slices.Index(total, pick), slices.Index(total, pick)+1)
	}
	for _, kw := range []string{"Flying", "Haste", "Trample"} {
		if got, wantKW := g.Card(creature).HasKeyword(kw), slices.Contains(want, kw); got != wantKW {
			t.Errorf("%s granted = %v, want %v (drawn %v)", kw, got, wantKW, want)
		}
	}
}

// TestPumpRandomKeywordNoRepetitionSkipsWhatTheTargetHas proves NoRepetition$
// (PumpEffect.java:385-391): a keyword the target already carries is out of
// the draw, so with one left no number is drawn and it is granted.
func TestPumpRandomKeywordNoRepetitionSkipsWhatTheTargetHas(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGame(t)
	def := etbPumpTriggerDefParams(t, "Test No Repetition",
		`Defined$ Self | KW$ Flying & Haste | RandomKeyword$ True | NoRepetition$ True | Duration$ Permanent`, nil)
	def.Faces[0].Keywords = append(def.Faces[0].Keywords, "Flying")
	creature, err := castETBPump(t, g, p, def)
	if err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if !g.Card(creature).HasKeyword("Haste") {
		t.Error("creature lacks Haste, the only keyword left to draw")
	}
}
