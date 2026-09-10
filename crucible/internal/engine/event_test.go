package engine_test

import (
	"testing"
	"unsafe"

	"github.com/jczastkiewicz/crucible/internal/engine"
)

// recordingSink is a stub, not a mock: the seam is the interface, so a test
// passes in a different Sink (TEST-8).
type recordingSink struct{ events []engine.Event }

func (s *recordingSink) Emit(e engine.Event) { s.events = append(s.events, e) }

// Phase names are what card scripts write, and several carry spaces --
// `Phase$ End of Turn` is not an identifier. The lookup has to round-trip
// them exactly or a trigger restricted to a step never fires.
func TestPhaseNamesRoundTrip(t *testing.T) {
	t.Parallel()

	for _, name := range []string{
		"Untap", "Upkeep", "Draw", "Main1", "BeginCombat", "Declare Attackers",
		"Declare Blockers", "First Strike Damage", "Combat Damage", "EndCombat",
		"Main2", "End of Turn", "Cleanup",
	} {
		p, ok := engine.PhaseByName(name)
		if !ok {
			t.Errorf("%q is not a known phase", name)
			continue
		}
		if got := p.String(); got != name {
			t.Errorf("%q round-tripped to %q", name, got)
		}
	}
	if _, ok := engine.PhaseByName("Main"); ok {
		t.Error("an invented phase name resolved")
	}
}

// Combat is a contiguous range, which is what lets a trigger restricted to
// combat test a bound rather than list six steps.
func TestCombatPhaseRange(t *testing.T) {
	t.Parallel()

	in := []engine.PhaseType{
		engine.CombatBegin, engine.DeclareAttackers, engine.DeclareBlockers,
		engine.FirstStrikeDamage, engine.CombatDamage, engine.CombatEnd,
	}
	for _, p := range in {
		if !p.IsCombat() {
			t.Errorf("%s is not reported as combat", p)
		}
	}
	for _, p := range []engine.PhaseType{engine.Untap, engine.Main1, engine.Main2, engine.Cleanup} {
		if p.IsCombat() {
			t.Errorf("%s is reported as combat", p)
		}
	}
}

// The zero Event must not look like a real one, or a struct that was never
// filled in reads as a turn beginning.
func TestZeroEventIsNotAnEvent(t *testing.T) {
	t.Parallel()

	var e engine.Event
	if e.Kind != engine.EventNone {
		t.Errorf("zero event has kind %s", e.Kind)
	}
	if got := engine.EventKind(200).String(); got != "None" {
		t.Errorf("an out-of-range kind stringified to %q", got)
	}
}

// Flags are a mask, and Has means "all of these", not "any of these" -- a
// caller asking for combat deathtouch damage wants both.
func TestEventFlagsAreAConjunction(t *testing.T) {
	t.Parallel()

	f := engine.FlagCombat | engine.FlagDeathtouch
	if !f.Has(engine.FlagCombat) || !f.Has(engine.FlagDeathtouch) {
		t.Error("a set flag reads as unset")
	}
	if !f.Has(engine.FlagCombat | engine.FlagDeathtouch) {
		t.Error("Has of both flags failed when both are set")
	}
	if f.Has(engine.FlagCombat | engine.FlagOptional) {
		t.Error("Has returned true when only one of the two flags is set")
	}
	if (engine.EventFlags(0)).Has(engine.FlagCombat) {
		t.Error("an empty mask reported a flag as set")
	}
}

// A clone's sink discards, because the AI explores lines that never happened
// and a clone holding the real sink would record imagined casts as real.
func TestDiscardSinkDropsEverything(t *testing.T) {
	t.Parallel()

	var s engine.Sink = engine.DiscardSink{}
	s.Emit(engine.Event{Kind: engine.SpellCast})
	// Nothing to assert beyond not panicking and not retaining: the type has
	// no state, which is the property being pinned.
	if unsafe.Sizeof(engine.DiscardSink{}) != 0 {
		t.Error("DiscardSink carries state")
	}
}

// A recorder folds in place, so the order it sees is the order the engine
// emitted -- there is no queue that could reorder.
func TestSinkSeesEmissionOrder(t *testing.T) {
	t.Parallel()

	var s recordingSink
	kinds := []engine.EventKind{engine.TurnBegan, engine.SpellCast, engine.DamageDealt}
	for i, k := range kinds {
		s.Emit(engine.Event{Kind: k, Turn: uint16(i)})
	}
	if len(s.events) != len(kinds) {
		t.Fatalf("sink saw %d events, want %d", len(s.events), len(kinds))
	}
	for i, k := range kinds {
		if s.events[i].Kind != k || s.events[i].Turn != uint16(i) {
			t.Errorf("event %d is %s turn %d, want %s turn %d",
				i, s.events[i].Kind, s.events[i].Turn, k, i)
		}
	}
}

// The record is fixed-size and pointer-free. At 10^5 games the stream is the
// largest thing the runner produces, so an allocation per event would be the
// cost that matters.
func TestEventRecordStaysFlat(t *testing.T) {
	t.Parallel()

	if got := unsafe.Sizeof(engine.Event{}); got > 40 {
		t.Errorf("Event is %d bytes; it is meant to stay a small flat record", got)
	}
	if engine.SchemaVersion != 1 {
		t.Errorf("SchemaVersion is %d; bumping it is a deliberate act with a reader change behind it",
			engine.SchemaVersion)
	}
}
