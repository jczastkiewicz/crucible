package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
	"github.com/jczastkiewicz/crucible/internal/mana"
)

// Add puts mana in by color, and Total counts every color plus colorless
// together.
func TestPoolAddAndTotal(t *testing.T) {
	t.Parallel()

	var p engine.Pool
	p.Add(mana.White, 2)
	p.Add(mana.Blue, 1)
	p.AddColorless(3)

	if got := p.Total(); got != 6 {
		t.Errorf("Total() = %d, want 6", got)
	}
}

// Add wants exactly one color -- the zero value or more than one bit set is
// an engine invariant breach, not something a card script can cause, so it
// panics rather than silently doing nothing (GO-7).
func TestPoolAddRequiresExactlyOneColor(t *testing.T) {
	t.Parallel()

	defer func() {
		if recover() == nil {
			t.Error("Add(White|Blue, ...) did not panic")
		}
	}()

	var p engine.Pool
	p.Add(mana.White|mana.Blue, 1)
}

// Empty clears every color and colorless together -- CR 500.4.
func TestPoolEmpty(t *testing.T) {
	t.Parallel()

	var p engine.Pool
	p.Add(mana.White, 2)
	p.AddColorless(1)
	p.Empty()

	if got := p.Total(); got != 0 {
		t.Errorf("Total() after Empty() = %d, want 0", got)
	}
}

// Pay spends exactly what a plain colored-and-generic cost asks for.
func TestPoolPayPlainCost(t *testing.T) {
	t.Parallel()

	var p engine.Pool
	p.Add(mana.White, 1)
	p.AddColorless(2)

	if !p.Pay(mana.MustParse("2 W")) {
		t.Fatal("Pay(2 W) failed with exactly enough mana")
	}
	if got := p.Total(); got != 0 {
		t.Errorf("Total() after paying exactly = %d, want 0", got)
	}
}

// Generic is paid from whatever color is left over, not just colorless.
func TestPoolPayGenericFromAnyLeftoverColor(t *testing.T) {
	t.Parallel()

	var p engine.Pool
	p.Add(mana.Blue, 2)

	if !p.Pay(mana.MustParse("2")) {
		t.Fatal("Pay({2}) failed with 2 floating blue mana")
	}
}

// A {C} pip needs colorless mana specifically -- five colored mana does not
// substitute for it, the same way a colored pip does not accept the wrong
// color.
func TestPoolPayColorlessPipNeedsColorlessSpecifically(t *testing.T) {
	t.Parallel()

	var p engine.Pool
	p.Add(mana.White, 5)

	if p.Pay(mana.MustParse("C")) {
		t.Fatal("Pay(C) succeeded paying a colorless pip with white mana")
	}
}

// Insufficient mana fails, and leaves the pool exactly as it was -- Pay
// never spends part of a cost it cannot finish paying.
func TestPoolPayInsufficientManaLeavesPoolUnchanged(t *testing.T) {
	t.Parallel()

	var p engine.Pool
	p.Add(mana.White, 1)

	if p.Pay(mana.MustParse("2 W")) {
		t.Fatal("Pay(2 W) succeeded with only 1 white mana")
	}
	if got := p.Total(); got != 1 {
		t.Errorf("pool changed after a failed Pay: Total() = %d, want 1", got)
	}
}

// A shard shape this port has no substitution rule for -- hybrid,
// Phyrexian, X, snow -- fails Pay even with plenty of mana on hand: the
// pool has no way to know which color a hybrid symbol should take or
// whether a Phyrexian symbol is being paid with life instead, so it reports
// the same "cannot resolve this" answer insufficient mana would.
func TestPoolPayUnresolvableShardIsGap(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name string
		cost string
	}{
		{"hybrid", "1 G W/U"},
		{"phyrexian", "2 W/P"},
		{"X", "X R"},
		{"snow", "1 S"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var p engine.Pool
			p.Add(mana.White, 5)
			p.Add(mana.Blue, 5)
			p.Add(mana.Green, 5)
			p.Add(mana.Red, 5)
			p.AddColorless(5)

			if p.Pay(mana.MustParse(tc.cost)) {
				t.Errorf("Pay(%s) succeeded despite an unresolvable shard", tc.cost)
			}
		})
	}
}

// CR 500.4: every player's floating mana empties on every phase/step
// transition, not just the active player's.
func TestEmptyManaPoolsClearsEveryPlayer(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	a, b := g.Players()[0], g.Players()[1]
	g.Player(a).ManaPool.Add(mana.White, 3)
	g.Player(b).ManaPool.AddColorless(2)
	g.SetTurnState(1, a, engine.Main1)

	g.AdvancePhase(engine.NewScriptedController())

	if got := g.Player(a).ManaPool.Total(); got != 0 {
		t.Errorf("active player's pool after AdvancePhase: Total() = %d, want 0", got)
	}
	if got := g.Player(b).ManaPool.Total(); got != 0 {
		t.Errorf("other player's pool after AdvancePhase: Total() = %d, want 0", got)
	}
}
