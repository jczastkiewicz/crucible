package engine_test

import (
	"errors"
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
)

// countingEffect is a stub, not a mock: dispatch is tested by passing in a
// different Effect, which is the seam the design already has (TEST-8).
type countingEffect struct {
	calls int
	err   error
}

func (e *countingEffect) Resolve(*engine.Game, *engine.Ability) error {
	e.calls++
	return e.err
}

// The API set is generated from Forge's enum, so it has to match it in count
// and resolve by the name a script writes.
func TestAPINamesMatchForge(t *testing.T) {
	t.Parallel()

	if got := engine.NumAPIs(); got < 190 {
		t.Fatalf("%d APIs generated; the enum's shape changed", got)
	}
	for _, name := range []string{"DealDamage", "ChangeZone", "Pump", "Draw", "Token"} {
		api, ok := engine.APIByName(name)
		if !ok {
			t.Errorf("%q is not a known API", name)
			continue
		}
		if got := api.String(); got != name {
			t.Errorf("%q round-tripped to %q", name, got)
		}
	}
	if _, ok := engine.APIByName("NotAnAPI"); ok {
		t.Error("an invented name resolved to an API")
	}
}

// A registered effect is dispatched to; an unregistered one is an error
// naming the API, not a nil dereference. Most of the array is empty
// throughout the port, so the empty case is the common one.
func TestRegistryDispatch(t *testing.T) {
	t.Parallel()

	draw, ok := engine.APIByName("Draw")
	if !ok {
		t.Fatal("Draw is not a known API")
	}
	mill, ok := engine.APIByName("Mill")
	if !ok {
		t.Fatal("Mill is not a known API")
	}

	var reg engine.Registry
	stub := &countingEffect{}
	reg[draw] = stub

	if err := reg.Resolve(nil, &engine.Ability{API: draw}); err != nil {
		t.Errorf("resolving a registered API: %v", err)
	}
	if stub.calls != 1 {
		t.Errorf("effect called %d times, want 1", stub.calls)
	}

	err := reg.Resolve(nil, &engine.Ability{API: mill})
	if !errors.Is(err, engine.ErrUnimplemented) {
		t.Errorf("unregistered API gave %v, want ErrUnimplemented", err)
	}
	// The diagnostic has to name the API, or a gap during the port is a
	// message nobody can act on.
	if err != nil && !contains(err.Error(), "Mill") {
		t.Errorf("error %q does not name the API", err)
	}

	if got := reg.Implemented(); got != 1 {
		t.Errorf("Implemented() = %d, want 1", got)
	}
}

// An out-of-range API is an error on the same terms, because it reaches the
// registry from data rather than from a constant.
func TestRegistryRejectsUnknownAPI(t *testing.T) {
	t.Parallel()

	var reg engine.Registry
	bad := engine.APIType(engine.NumAPIs() + 5)
	if err := reg.Resolve(nil, &engine.Ability{API: bad}); !errors.Is(err, engine.ErrUnimplemented) {
		t.Errorf("out-of-range API gave %v, want ErrUnimplemented", err)
	}
	if got := bad.String(); got == "" {
		t.Error("an out-of-range API stringified to nothing")
	}
}

// An effect's own error reaches the caller unchanged: a card that fails is a
// game that fails, not a batch that dies (GO-7).
func TestEffectErrorPropagates(t *testing.T) {
	t.Parallel()

	sentinel := errors.New("boom")
	api, _ := engine.APIByName("Draw")
	var reg engine.Registry
	reg[api] = &countingEffect{err: sentinel}

	if err := reg.Resolve(nil, &engine.Ability{API: api}); !errors.Is(err, sentinel) {
		t.Errorf("got %v, want the effect's own error", err)
	}
}

func contains(haystack, needle string) bool {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}
