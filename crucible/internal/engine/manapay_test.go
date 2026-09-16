package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
	"github.com/jczastkiewicz/crucible/internal/mana"
)

// A two-colour hybrid shard pays from either color the pool has -- CR
// 601.2h, resolved by asking the controller rather than picking one
// automatically.
func TestPayManaCostResolvesTwoColorHybridFromController(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a")
	p := g.Players()[0]
	g.Player(p).ManaPool.Add(mana.Blue, 1)
	c := engine.NewScriptedController()
	c.QueueHybridManaColor(mana.Blue)

	if !g.PayManaCost(p, mana.MustParse("W/U"), c) {
		t.Fatal("PayManaCost failed paying {W/U} with a blue mana in the pool")
	}
	if got := g.Player(p).ManaPool.Total(); got != 0 {
		t.Errorf("pool total after payment = %d, want 0", got)
	}
}

// The controller's answer decides which color is spent -- choosing white
// when the pool holds white but not blue must fail, not silently fall back
// to whatever the pool actually has.
func TestPayManaCostFailsWhenTheChosenColorIsNotInThePool(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a")
	p := g.Players()[0]
	g.Player(p).ManaPool.Add(mana.White, 1)
	c := engine.NewScriptedController()
	c.QueueHybridManaColor(mana.Blue)

	if g.PayManaCost(p, mana.MustParse("W/U"), c) {
		t.Fatal("PayManaCost succeeded choosing blue with only white in the pool")
	}
	if got := g.Player(p).ManaPool.Total(); got != 1 {
		t.Errorf("pool total after a failed payment = %d, want 1 (unchanged)", got)
	}
}

// A cost mixing a plain shard and a hybrid shard resolves the hybrid and
// pays the plain shard the same call -- PayManaCost is one payment, not a
// per-shard loop the caller has to drive.
func TestPayManaCostResolvesHybridAlongsidePlainShards(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a")
	p := g.Players()[0]
	g.Player(p).ManaPool.Add(mana.Red, 1)
	g.Player(p).ManaPool.Add(mana.Green, 1)
	c := engine.NewScriptedController()
	c.QueueHybridManaColor(mana.Green)

	if !g.PayManaCost(p, mana.MustParse("R G/W"), c) {
		t.Fatal("PayManaCost failed paying {R}{G/W} with red and green in the pool")
	}
	if got := g.Player(p).ManaPool.Total(); got != 0 {
		t.Errorf("pool total after payment = %d, want 0", got)
	}
}

// A monocoloured hybrid ({2/W}) is not a two-colour hybrid -- it passes
// through unresolved and Pool.Pay's own unresolvable-shard branch fails the
// payment, the same as calling Pay directly would.
func TestPayManaCostStillFailsForAMonocoloredHybrid(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a")
	p := g.Players()[0]
	g.Player(p).ManaPool.AddColorless(2)
	c := engine.NewScriptedController()

	if g.PayManaCost(p, mana.MustParse("2/W"), c) {
		t.Fatal("PayManaCost succeeded on a monocolored hybrid shard, which is not yet resolved")
	}
}

// A colourless hybrid ({C/W}) has exactly one colour bit plus the colorless
// atom -- Colors().Count() == 1, so it is not mistaken for a two-colour
// hybrid either.
func TestPayManaCostStillFailsForAColorlessHybrid(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a")
	p := g.Players()[0]
	g.Player(p).ManaPool.AddColorless(1)
	c := engine.NewScriptedController()

	if g.PayManaCost(p, mana.MustParse("C/W"), c) {
		t.Fatal("PayManaCost succeeded on a colorless hybrid shard, which is not yet resolved")
	}
}

// Phyrexian mana ({W/P}) has one colour plus the or-2-life atom -- also not
// a two-colour hybrid, and still unresolved.
func TestPayManaCostStillFailsForPhyrexianMana(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a")
	p := g.Players()[0]
	g.Player(p).Life = 20
	c := engine.NewScriptedController()

	if g.PayManaCost(p, mana.MustParse("W/P"), c) {
		t.Fatal("PayManaCost succeeded on a Phyrexian shard, which is not yet resolved")
	}
	if g.Player(p).Life != 20 {
		t.Errorf("life = %d, want unchanged at 20 -- no life-payment path exists yet", g.Player(p).Life)
	}
}
