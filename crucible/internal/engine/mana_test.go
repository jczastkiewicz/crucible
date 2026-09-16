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

// Breakdown reports every type's own count, in a fixed white/blue/black/red
// /green/colorless order -- Total alone cannot tell two differently-composed
// pools apart.
func TestPoolBreakdown(t *testing.T) {
	t.Parallel()

	var p engine.Pool
	p.Add(mana.White, 1)
	p.Add(mana.Red, 2)
	p.AddColorless(3)

	if got, want := p.Breakdown(), [6]int{1, 0, 0, 2, 0, 3}; got != want {
		t.Errorf("Breakdown() = %v, want %v", got, want)
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

// Empty clears every color and colorless together -- CR 500.4 -- snow
// buckets included.
func TestPoolEmpty(t *testing.T) {
	t.Parallel()

	var p engine.Pool
	p.Add(mana.White, 2)
	p.AddColorless(1)
	p.AddSnow(mana.Blue, 3)
	p.AddSnowColorless(1)
	p.Empty()

	if got := p.Total(); got != 0 {
		t.Errorf("Total() after Empty() = %d, want 0", got)
	}
}

// AddSnow puts mana in the snow half of a color's bucket -- Breakdown sees
// it summed into that color's total (CR 106.3a: snow mana is still that
// color), and SnowBreakdown sees only the snow half.
func TestPoolAddSnowBreakdownAndSnowBreakdown(t *testing.T) {
	t.Parallel()

	var p engine.Pool
	p.Add(mana.White, 1)
	p.AddSnow(mana.White, 2)
	p.AddSnow(mana.Red, 1)
	p.AddSnowColorless(3)

	if got, want := p.Breakdown(), [6]int{3, 0, 0, 1, 0, 3}; got != want {
		t.Errorf("Breakdown() = %v, want %v", got, want)
	}
	if got, want := p.SnowBreakdown(), [6]int{2, 0, 0, 1, 0, 3}; got != want {
		t.Errorf("SnowBreakdown() = %v, want %v", got, want)
	}
	if got, want := p.Total(), 7; got != want {
		t.Errorf("Total() = %d, want %d", got, want)
	}
}

// AddSnow wants exactly one color, the same invariant Add carries.
func TestPoolAddSnowRequiresExactlyOneColor(t *testing.T) {
	t.Parallel()

	defer func() {
		if recover() == nil {
			t.Error("AddSnow(White|Blue, ...) did not panic")
		}
	}()

	var p engine.Pool
	p.AddSnow(mana.White|mana.Blue, 1)
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

// A plain colored pip falls back to that color's snow bucket once the plain
// one is empty -- CR 106.3a: snow mana is still that color, so it pays a
// same-color pip exactly as plain mana does.
func TestPoolPayFallsBackToSnowForAColoredPip(t *testing.T) {
	t.Parallel()

	var p engine.Pool
	p.AddSnow(mana.White, 1)

	if !p.Pay(mana.MustParse("W")) {
		t.Fatal("Pay(W) failed against a pool holding only snow white")
	}
	if got := p.Total(); got != 0 {
		t.Errorf("Total() after payment = %d, want 0", got)
	}
}

// Generic falls back to snow the same way, after every plain bucket the
// fixed order already tries is empty.
func TestPoolPayFallsBackToSnowForGeneric(t *testing.T) {
	t.Parallel()

	var p engine.Pool
	p.AddSnowColorless(1)
	p.AddSnow(mana.Red, 1)

	if !p.Pay(mana.MustParse("2")) {
		t.Fatal("Pay({2}) failed against a pool holding only snow colorless and snow red")
	}
	if got := p.Total(); got != 0 {
		t.Errorf("Total() after payment = %d, want 0", got)
	}
}

// PayWithSnow spends the named color's own snow bucket for each entry in
// snow, never the plain one -- even when plain mana of that color is sitting
// right there in the same pool.
func TestPoolPayWithSnowSpendsTheSnowBucketNotThePlainOne(t *testing.T) {
	t.Parallel()

	var p engine.Pool
	p.Add(mana.White, 1)
	p.AddSnow(mana.White, 1)

	if !p.PayWithSnow(mana.Cost{}, []mana.Shard{mana.ShardW}) {
		t.Fatal("PayWithSnow failed with one snow white available")
	}
	if got, want := p.Breakdown(), [6]int{1, 0, 0, 0, 0, 0}; got != want {
		t.Errorf("Breakdown() after payment = %v, want %v (plain white untouched)", got, want)
	}
	if got, want := p.SnowBreakdown(), [6]int{0, 0, 0, 0, 0, 0}; got != want {
		t.Errorf("SnowBreakdown() after payment = %v, want %v (snow white spent)", got, want)
	}
}

// A snow entry fails when its color has no snow mana, even if plenty of
// plain mana of that same color is available -- plain mana never covers a
// snow requirement.
func TestPoolPayWithSnowFailsWhenColorHasNoSnowMana(t *testing.T) {
	t.Parallel()

	var p engine.Pool
	p.Add(mana.White, 5)

	if p.PayWithSnow(mana.Cost{}, []mana.Shard{mana.ShardW}) {
		t.Fatal("PayWithSnow succeeded spending snow white with only plain white in the pool")
	}
	if got := p.Total(); got != 5 {
		t.Errorf("Total() after a failed payment = %d, want 5 (unchanged)", got)
	}
}

// A snow spend that succeeds is still rolled back if a later part of the
// same payment fails -- PayWithSnow is all-or-nothing across cost and snow
// together, the same guarantee Pay itself already makes for cost alone.
func TestPoolPayWithSnowRollsBackOnLaterFailure(t *testing.T) {
	t.Parallel()

	var p engine.Pool
	p.AddSnow(mana.White, 1)

	if p.PayWithSnow(mana.MustParse("U"), []mana.Shard{mana.ShardW}) {
		t.Fatal("PayWithSnow succeeded despite no blue mana for the cost's own {U} pip")
	}
	if got, want := p.SnowBreakdown(), [6]int{1, 0, 0, 0, 0, 0}; got != want {
		t.Errorf("SnowBreakdown() after a failed payment = %v, want %v (the snow white spend rolled back)", got, want)
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
