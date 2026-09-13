package engine_test

import (
	"errors"
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
)

// order is a stub Effect that records the sequence it was called in, the way
// effect_test.go's countingEffect records a count -- the same stub-not-mock
// seam (TEST-8).
type order struct {
	got []engine.CardID
	err error
}

func (o *order) Resolve(g *engine.Game, a *engine.Ability) error {
	o.got = append(o.got, a.Source)
	return o.err
}

// CR 405: the stack is empty until something is pushed, and StackTop reports
// nothing rather than a zero Ability that could be mistaken for a real one.
func TestStackStartsEmpty(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a")
	if got := g.StackLen(); got != 0 {
		t.Errorf("StackLen() = %d, want 0", got)
	}
	if _, ok := g.StackTop(); ok {
		t.Error("StackTop() reported an ability on an empty stack")
	}
}

// CR 405.1: pushing puts an ability on top, growing the count and replacing
// whatever StackTop reported before.
func TestPushAbilityGrowsStackAndSetsTop(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a")
	p := g.Players()[0]
	first, second := engine.CardID(1), engine.CardID(2)

	g.PushAbility(engine.Ability{Source: first, Controller: p})
	if got := g.StackLen(); got != 1 {
		t.Fatalf("StackLen() = %d, want 1", got)
	}
	top, ok := g.StackTop()
	if !ok || top.Source != first {
		t.Errorf("StackTop() = (%+v, %v), want source %d", top, ok, first)
	}

	g.PushAbility(engine.Ability{Source: second, Controller: p})
	if got := g.StackLen(); got != 2 {
		t.Errorf("StackLen() = %d, want 2", got)
	}
	top, ok = g.StackTop()
	if !ok || top.Source != second {
		t.Errorf("StackTop() = (%+v, %v), want source %d (the more recent push)", top, ok, second)
	}
}

// PushAbility emits AbilityActivated, the pair AbilityResolved forms once the
// item resolves.
func TestPushAbilityEmitsAbilityActivated(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a")
	p := g.Players()[0]
	var sink recordingSink
	g.SetSink(&sink)

	g.PushAbility(engine.Ability{Source: 5, Controller: p})

	if len(sink.events) != 1 || sink.events[0].Kind != engine.AbilityActivated {
		t.Fatalf("events = %+v, want one AbilityActivated", sink.events)
	}
	if sink.events[0].Source != 5 || sink.events[0].Actor != p {
		t.Errorf("event source/actor = %d/%d, want 5/%d", sink.events[0].Source, sink.events[0].Actor, p)
	}
}

// CR 405.5: the stack resolves last on, first off, dispatching through the
// registered Effect -- proven by registering a stub under an API and reading
// its call order back.
func TestResolveStackDispatchesLIFOThroughRegistry(t *testing.T) {
	t.Parallel()

	// Two players, both above the loss threshold: a single-player game
	// satisfies CR 104.2a's "one player left standing" trivially, so
	// CheckStateBasedActions (run after each resolve) would end it on the
	// first pass regardless of life.
	g := newGame(t, "a", "b")
	p := g.Players()[0]
	g.Player(p).Life, g.Player(g.Players()[1]).Life = 20, 20
	api, ok := engine.APIByName("Draw")
	if !ok {
		t.Fatal("Draw is not a known API")
	}

	stub := &order{}
	var reg engine.Registry
	reg[api] = stub

	for _, id := range []engine.CardID{1, 2, 3} {
		g.PushAbility(engine.Ability{API: api, Source: id, Controller: p})
	}
	if err := g.ResolveStack(&reg); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}

	want := []engine.CardID{3, 2, 1}
	if len(stub.got) != len(want) {
		t.Fatalf("resolved %v, want %v", stub.got, want)
	}
	for i := range want {
		if stub.got[i] != want[i] {
			t.Errorf("resolve order[%d] = %d, want %d", i, stub.got[i], want[i])
		}
	}
	if got := g.StackLen(); got != 0 {
		t.Errorf("StackLen() after resolving = %d, want 0", got)
	}
}

// Each resolution emits AbilityResolved before the next one is popped.
func TestResolveStackEmitsAbilityResolvedPerItem(t *testing.T) {
	t.Parallel()

	// Two players for the same reason TestResolveStackDispatchesLIFOThroughRegistry
	// uses two: a single-player game ends itself on the first
	// CheckStateBasedActions call regardless of life.
	g := newGame(t, "a", "b")
	p := g.Players()[0]
	g.Player(p).Life, g.Player(g.Players()[1]).Life = 20, 20
	var sink recordingSink
	g.SetSink(&sink)

	api, ok := engine.APIByName("Draw")
	if !ok {
		t.Fatal("Draw is not a known API")
	}
	var reg engine.Registry
	reg[api] = &order{}

	g.PushAbility(engine.Ability{API: api, Source: 1, Controller: p})
	g.PushAbility(engine.Ability{API: api, Source: 2, Controller: p})

	if err := g.ResolveStack(&reg); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}

	var resolved []engine.CardID
	for _, e := range sink.events {
		if e.Kind == engine.AbilityResolved {
			resolved = append(resolved, e.Source)
		}
	}
	want := []engine.CardID{2, 1}
	if len(resolved) != len(want) {
		t.Fatalf("AbilityResolved events for %v, want %v", resolved, want)
	}
	for i := range want {
		if resolved[i] != want[i] {
			t.Errorf("AbilityResolved[%d] source = %d, want %d", i, resolved[i], want[i])
		}
	}
}

// GO-7: an effect's own error stops the loop immediately and reaches the
// caller unchanged, leaving whatever was still under it on the stack rather
// than resolving it as if nothing happened.
func TestResolveStackStopsOnEffectError(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a")
	p := g.Players()[0]
	api, ok := engine.APIByName("Draw")
	if !ok {
		t.Fatal("Draw is not a known API")
	}
	sentinel := errors.New("boom")
	var reg engine.Registry
	reg[api] = &order{err: sentinel}

	g.PushAbility(engine.Ability{API: api, Source: 1, Controller: p})
	g.PushAbility(engine.Ability{API: api, Source: 2, Controller: p})

	err := g.ResolveStack(&reg)
	if !errors.Is(err, sentinel) {
		t.Fatalf("ResolveStack() = %v, want the effect's own error", err)
	}
	if got := g.StackLen(); got != 1 {
		t.Errorf("StackLen() after the error = %d, want 1 (the item under the failed one)", got)
	}
}

// An unregistered API is ErrUnimplemented, the same as calling Registry.Resolve
// directly -- the stack does not swallow or reinterpret dispatch's own error.
func TestResolveStackPropagatesUnimplemented(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a")
	p := g.Players()[0]
	var reg engine.Registry
	g.PushAbility(engine.Ability{Controller: p})

	if err := g.ResolveStack(&reg); !errors.Is(err, engine.ErrUnimplemented) {
		t.Errorf("ResolveStack() = %v, want ErrUnimplemented", err)
	}
}

// CR 704.3/704: if a resolution ends the game, nothing else on the stack
// resolves afterward -- the same short-circuit CheckStateBasedActions itself
// already applies once g.Over() is true.
func TestResolveStackStopsWhenGameEnds(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	a, b := g.Players()[0], g.Players()[1]
	g.Player(a).Life, g.Player(b).Life = 20, 20

	api, ok := engine.APIByName("Draw")
	if !ok {
		t.Fatal("Draw is not a known API")
	}
	var reg engine.Registry
	reg[api] = &endsGame{loser: a}

	g.PushAbility(engine.Ability{API: api, Source: 1, Controller: a})
	g.PushAbility(engine.Ability{API: api, Source: 2, Controller: a})

	if err := g.ResolveStack(&reg); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if !g.Over() {
		t.Fatal("setup: game did not end")
	}
	if got := g.StackLen(); got != 1 {
		t.Errorf("StackLen() after the game ended mid-resolution = %d, want 1 (unresolved)", got)
	}
}

// endsGame is a stub Effect that sets its loser to zero life, so resolving it
// makes CheckStateBasedActions end the game -- proving ResolveStack's loop
// actually reads g.Over() between items rather than draining the stack
// unconditionally.
type endsGame struct{ loser engine.PlayerID }

func (e *endsGame) Resolve(g *engine.Game, a *engine.Ability) error {
	g.Player(e.loser).Life = 0
	return nil
}

// A cloned game's stack is its own slice: pushing onto the clone must not
// write back to the original, the same sharing bug Counters, Memory and
// attachments were already guarded against.
func TestCloneCopiesStack(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a")
	p := g.Players()[0]
	g.PushAbility(engine.Ability{Source: 1, Controller: p})

	c := g.Clone()
	c.PushAbility(engine.Ability{Source: 2, Controller: p})

	if got := g.StackLen(); got != 1 {
		t.Errorf("original StackLen() = %d after the clone's changed, want 1", got)
	}
	if got := c.StackLen(); got != 2 {
		t.Errorf("clone StackLen() = %d, want 2", got)
	}
}
