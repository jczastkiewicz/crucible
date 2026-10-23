package engine

// package engine, not engine_test (TEST-2's "invariant not observable from
// outside" row): these helpers sit behind resolvers that only the engine
// builds; each case pins the value the Java counterpart returns.

import (
	"slices"
	"testing"

	"github.com/jczastkiewicz/crucible/internal/carddb/compile"
	"github.com/jczastkiewicz/crucible/internal/mana"
)

func TestTriggerCountMaxIsTheHighestRecordedResult(t *testing.T) {
	t.Parallel()

	counts := triggerCounts{results: []int{2, 5, 3}, set: countResult}
	if n, ok := counts.countMax("Result"); !ok || n != 5 {
		t.Errorf("countMax(Result) = %d, %v; want 5, true", n, ok)
	}
	if n, ok := counts.countMax("Other"); ok || n != 0 {
		t.Errorf("countMax(Other) = %d, %v; want 0, false", n, ok)
	}
	if n, ok := (triggerCounts{}).countMax("Result"); ok || n != 0 {
		t.Errorf("countMax on an unrecorded roll = %d, %v; want 0, false", n, ok)
	}

	g := targetCandidatesTestGame(t, "a", "b")
	src := g.NewCard(targetCandidatesTestCreature(t), g.Players()[0], Battlefield)
	if _, ok := g.triggerCountMax(src, "Result"); ok {
		t.Error("triggerCountMax resolved with no resolving ability")
	}
	g.resolving = &Ability{Source: src, triggered: triggeredObjects{counts: counts}}
	if n, ok := g.triggerCountMax(src, "Result"); !ok || n != 5 {
		t.Errorf("triggerCountMax = %d, %v; want 5, true", n, ok)
	}
	if _, ok := g.triggerCountMax(NoCard, "Result"); ok {
		t.Error("triggerCountMax resolved for no source")
	}
}

func TestControlModTruncateUndoesATrialApplication(t *testing.T) {
	t.Parallel()

	var m ControlMod
	m.effects = []ControlEffect{{}, {}}
	mark := m.size()
	m.effects = append(m.effects, ControlEffect{})
	if m.size() != 3 {
		t.Fatalf("size = %d, want 3", m.size())
	}
	m.truncate(mark)
	if m.size() != 2 {
		t.Errorf("size after truncate = %d, want 2", m.size())
	}
	if c := m.clone(); c.size() != 2 {
		t.Errorf("clone size = %d, want 2", c.size())
	}
}

func TestSmallListHelpers(t *testing.T) {
	t.Parallel()

	if !containsColors([]mana.Colors{mana.Red, mana.Green}, mana.Green) || containsColors(nil, mana.Red) {
		t.Error("containsColors answered wrong")
	}
	clone := charmModeLists{turn: []string{"a"}, game: []string{"b"}}.clone()
	clone.turn[0] = "z"
	if slices.Equal(clone.turn, []string{"a"}) {
		t.Error("charmModeLists.clone shares its backing array")
	}
	DiscardSink{}.Emit(Event{})

	var p Player
	p.noteNumber("x", 3)
	p.noteNumber("x", 4)
	p.noteNumber("y", 1)
	if p.notedNumber("x") != 4 || p.notedNumber("y") != 1 || p.notedNumber("none") != 0 {
		t.Errorf("noted numbers = %v, want x=4 y=1 none=0", p.notedNumbers)
	}

	got := layerKeywordPerColorAll([]string{"Protection from ChosenColor", "Flying"}, mana.Red)
	if len(got) == 0 || !slices.Contains(got, "Flying") {
		t.Errorf("layerKeywordPerColorAll = %v, want the per-color keyword and Flying", got)
	}
	var f compile.Face
	f.HasColors, f.Colors = true, mana.Blue
	if faceColors(f) != mana.Blue {
		t.Error("faceColors ignored the Colors: override")
	}
	f.HasColors = false
	f.ManaCost = mana.MustParse("R")
	if faceColors(f) != mana.Red {
		t.Error("faceColors ignored the mana cost")
	}
}

func TestAnyValidMatchesTakesASemicolonList(t *testing.T) {
	t.Parallel()

	g := targetCandidatesTestGame(t, "a", "b")
	p := g.Players()[0]
	c := g.NewCard(targetCandidatesTestCreature(t), p, Battlefield)
	if !anyValidMatches(g, g.Card(c), "Land;Creature", p, c) {
		t.Error("Creature in a Land;Creature list did not match")
	}
	if anyValidMatches(g, g.Card(c), "Land;Artifact", p, c) {
		t.Error("a creature matched Land;Artifact")
	}
}
