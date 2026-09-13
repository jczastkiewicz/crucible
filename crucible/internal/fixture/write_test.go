package fixture_test

import (
	"strings"
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
	"github.com/jczastkiewicz/crucible/internal/fixture"
)

// Write covers every global directive, not just the ones Dump happens to
// produce: a hand-built State exercises activephaseadvance,
// removesummoningsickness and ability strings directly, rather than relying
// on Dump ever choosing to set them.
func TestWriteEveryGlobalDirective(t *testing.T) {
	t.Parallel()

	st := &fixture.State{
		Turn:                    2,
		ActivePlayer:            "human",
		ActivePhase:             engine.Main1,
		Phased:                  true,
		ActivePhaseAdvance:      engine.CombatBegin,
		PhaseAdvanced:           true,
		RemoveSummoningSickness: true,
		AbilityStrings:          map[string]string{"2": "second", "1": "first"},
	}
	st.Players[0] = fixture.PlayerState{Named: true, Life: 20, Counters: "POISON=3"}

	var buf strings.Builder
	if err := fixture.Write(&buf, st); err != nil {
		t.Fatalf("Write: %v", err)
	}
	text := buf.String()

	for _, want := range []string{
		"turn=2\n", "activeplayer=human\n", "activephase=Main1\n",
		"activephaseadvance=BeginCombat\n", "removesummoningsickness=true\n",
		"humanlife=20\n", "humancounters=POISON=3\n", "ability1=first\n", "ability2=second\n",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("Write output missing %q; got:\n%s", want, text)
		}
	}

	got, err := fixture.Parse(strings.NewReader(text))
	if err != nil {
		t.Fatalf("Parse(Write(x)): %v", err)
	}
	if got.Turn != st.Turn || got.ActivePlayer != st.ActivePlayer {
		t.Errorf("round trip: turn=%d activeplayer=%q, want %d %q", got.Turn, got.ActivePlayer, st.Turn, st.ActivePlayer)
	}
	if !got.PhaseAdvanced || got.ActivePhaseAdvance != engine.CombatBegin {
		t.Errorf("round trip: phaseadvance=%v %v, want true CombatBegin", got.PhaseAdvanced, got.ActivePhaseAdvance)
	}
	if !got.RemoveSummoningSickness {
		t.Error("round trip: removesummoningsickness did not survive")
	}
	if got.AbilityStrings["1"] != "first" || got.AbilityStrings["2"] != "second" {
		t.Errorf("round trip: ability strings %v", got.AbilityStrings)
	}
	if got.Players[0].Counters != "POISON=3" {
		t.Errorf("round trip: human counters %q, want POISON=3", got.Players[0].Counters)
	}
}

// A State with nothing set writes nothing but is still valid input to Parse.
func TestWriteEmptyStateIsValid(t *testing.T) {
	t.Parallel()

	var buf strings.Builder
	if err := fixture.Write(&buf, &fixture.State{}); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if _, err := fixture.Parse(strings.NewReader(buf.String())); err != nil {
		t.Errorf("Parse(Write(empty)): %v", err)
	}
}
