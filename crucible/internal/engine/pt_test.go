package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
)

// The layer's own number is what Forge's StaticAbilityLayer.num carries,
// and what a future port log or diagnostic reads back.
func TestStaticAbilityLayerString(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		layer engine.StaticAbilityLayer
		want  string
	}{
		{engine.LayerCopy, "1"},
		{engine.LayerCharacteristic, "7a"},
		{engine.LayerSetPT, "7b"},
		{engine.LayerModifyPT, "7c"},
		{engine.LayerRules, "8"},
	} {
		if got := tc.layer.String(); got != tc.want {
			t.Errorf("%v.String() = %q, want %q", tc.layer, got, tc.want)
		}
	}
	if got := engine.StaticAbilityLayer(200).String(); got != "?" {
		t.Errorf("out-of-range layer stringified to %q, want \"?\"", got)
	}
}

// With no continuous effects and no counters, Power/Toughness just reads
// the printed value straight through.
func TestPowerToughnessNoEffects(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a")
	p := g.Players()[0]
	id := g.NewCard(creatureDefPT(t, "3", "4"), p, engine.Battlefield)

	if pw, ok := g.Card(id).Power(); !ok || pw != 3 {
		t.Errorf("Power() = (%d, %v), want (3, true)", pw, ok)
	}
	if tg, ok := g.Card(id).Toughness(); !ok || tg != 4 {
		t.Errorf("Toughness() = (%d, %v), want (4, true)", tg, ok)
	}
}

// LayerModifyPT effects stack additively onto the base -- a "gets +1/+1"
// and a "gets +2/+0" together leave +3/+1.
func TestPowerToughnessModifyPTStacks(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a")
	p := g.Players()[0]
	id := g.NewCard(creatureDefPT(t, "2", "2"), p, engine.Battlefield)
	c := g.Card(id)
	c.PT.Add(engine.PTEffect{Layer: engine.LayerModifyPT, Timestamp: 1, Power: 1, Toughness: 1})
	c.PT.Add(engine.PTEffect{Layer: engine.LayerModifyPT, Timestamp: 2, Power: 2, Toughness: 0})

	if pw, ok := c.Power(); !ok || pw != 5 {
		t.Errorf("Power() = (%d, %v), want (5, true)", pw, ok)
	}
	if tg, ok := c.Toughness(); !ok || tg != 3 {
		t.Errorf("Toughness() = (%d, %v), want (3, true)", tg, ok)
	}
}

// LayerSetPT replaces the running value outright, regardless of what the
// base was -- a "becomes 0/1" effect on a 4/4 leaves 0/1, not 4/5.
func TestPowerToughnessSetPTReplaces(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a")
	p := g.Players()[0]
	id := g.NewCard(creatureDefPT(t, "4", "4"), p, engine.Battlefield)
	c := g.Card(id)
	c.PT.Add(engine.PTEffect{Layer: engine.LayerSetPT, Timestamp: 1, Power: 0, Toughness: 1, HasPower: true, HasToughness: true})

	if pw, ok := c.Power(); !ok || pw != 0 {
		t.Errorf("Power() = (%d, %v), want (0, true)", pw, ok)
	}
	if tg, ok := c.Toughness(); !ok || tg != 1 {
		t.Errorf("Toughness() = (%d, %v), want (1, true)", tg, ok)
	}
}

// A LayerSetPT effect naming only one dimension (a real corpus SetPower or
// SetToughness line without the other) leaves the other exactly as it was --
// not reset to zero, which HasPower/HasToughness (pt.go) exist to prevent.
func TestPowerToughnessSetPTPartialLeavesOtherDimensionAlone(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a")
	p := g.Players()[0]
	id := g.NewCard(creatureDefPT(t, "4", "4"), p, engine.Battlefield)
	c := g.Card(id)
	c.PT.Add(engine.PTEffect{Layer: engine.LayerSetPT, Timestamp: 1, Power: 0, HasPower: true})

	if pw, ok := c.Power(); !ok || pw != 0 {
		t.Errorf("Power() = (%d, %v), want (0, true)", pw, ok)
	}
	if tg, ok := c.Toughness(); !ok || tg != 4 {
		t.Errorf("Toughness() = (%d, %v), want (4, true) -- SetPower alone must not touch Toughness", tg, ok)
	}
}

// LayerSetPT applies before LayerModifyPT regardless of timestamp order --
// a "becomes 0/1" set with a later timestamp still applies before a "gets
// +1/+1" pump with an earlier one (CR 613.4's own sub-layer order, not
// insertion or timestamp order).
func TestPowerToughnessSetPTAppliesBeforeModifyPT(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a")
	p := g.Players()[0]
	id := g.NewCard(creatureDefPT(t, "4", "4"), p, engine.Battlefield)
	c := g.Card(id)
	c.PT.Add(engine.PTEffect{Layer: engine.LayerModifyPT, Timestamp: 1, Power: 1, Toughness: 1})
	c.PT.Add(engine.PTEffect{Layer: engine.LayerSetPT, Timestamp: 2, Power: 0, Toughness: 1, HasPower: true, HasToughness: true})

	if pw, ok := c.Power(); !ok || pw != 1 {
		t.Errorf("Power() = (%d, %v), want (1, true): set to 0 first, then +1", pw, ok)
	}
}

// A LayerCharacteristic effect supplies a value even when the printed one
// is unresolvable -- the whole point of a characteristic-defining ability.
func TestPowerToughnessCharacteristicResolvesUnresolvableBase(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a")
	p := g.Players()[0]
	id := g.NewCard(creatureDefPT(t, "*", "*"), p, engine.Battlefield)
	c := g.Card(id)

	if _, ok := c.Power(); ok {
		t.Fatal("setup: an unresolvable base resolved before any effect was added")
	}

	c.PT.Add(engine.PTEffect{Layer: engine.LayerCharacteristic, Timestamp: 1, Power: 2, Toughness: 2, HasPower: true, HasToughness: true})

	if pw, ok := c.Power(); !ok || pw != 2 {
		t.Errorf("Power() = (%d, %v), want (2, true)", pw, ok)
	}
	if tg, ok := c.Toughness(); !ok || tg != 2 {
		t.Errorf("Toughness() = (%d, %v), want (2, true)", tg, ok)
	}
}

// +1/+1 and -1/-1 counters fold in after every layer (CR 613.4), on top of
// whatever Layer 7 already produced.
func TestPowerToughnessCounters(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a")
	p := g.Players()[0]
	id := g.NewCard(creatureDefPT(t, "2", "2"), p, engine.Battlefield)
	c := g.Card(id)
	c.Counters.Add(engine.P1P1, 3)
	c.Counters.Add(engine.M1M1, 1)

	if pw, ok := c.Power(); !ok || pw != 4 {
		t.Errorf("Power() = (%d, %v), want (4, true): 2 base + 3 - 1", pw, ok)
	}
}

// Move clears PT the moment a card leaves the battlefield -- a continuous
// effect that only applied there does not survive the move.
func TestMoveClearsPT(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a")
	p := g.Players()[0]
	id := g.NewCard(creatureDefPT(t, "2", "2"), p, engine.Battlefield)
	g.Card(id).PT.Add(engine.PTEffect{Layer: engine.LayerModifyPT, Timestamp: 1, Power: 5, Toughness: 5})

	if pw, _ := g.Card(id).Power(); pw != 7 {
		t.Fatalf("setup: Power() = %d, want 7", pw)
	}

	g.Move(id, engine.Graveyard, p)

	if pw, ok := g.Card(id).Power(); !ok || pw != 2 {
		t.Errorf("Power() after leaving the battlefield = (%d, %v), want (2, true) -- the PT effect should be gone", pw, ok)
	}
}

// A cloned game's PT effects are its own slice: adding to the clone must
// not write back to the original.
func TestCloneCopiesPT(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a")
	p := g.Players()[0]
	id := g.NewCard(creatureDefPT(t, "2", "2"), p, engine.Battlefield)

	c := g.Clone()
	c.Card(id).PT.Add(engine.PTEffect{Layer: engine.LayerModifyPT, Timestamp: 1, Power: 5, Toughness: 5})

	if pw, _ := g.Card(id).Power(); pw != 2 {
		t.Errorf("original Power() = %d after the clone's changed, want 2", pw)
	}
	if pw, _ := c.Card(id).Power(); pw != 7 {
		t.Errorf("clone Power() = %d, want 7", pw)
	}
}
