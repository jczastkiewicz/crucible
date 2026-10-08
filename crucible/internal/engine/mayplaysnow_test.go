package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
	"github.com/jczastkiewicz/crucible/internal/mana"
)

// TestMayPlaySnowIgnoreColorLetsSnowManaPayAColoredCost proves
// MayPlaySnowIgnoreColor$ (CardPlayOption.isIgnoreSnowSourceManaCostColor,
// ManaCostBeingPaid.canBePaidWith): snow mana of any type, colorless included,
// pays a colored part of the cost, plain mana still pays only its own color,
// and a {C} part gets no relief. Without the param snow mana is just its color.
func TestMayPlaySnowIgnoreColorLetsSnowManaPayAColoredCost(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name   string
		params string
		cost   string
		pool   func(*engine.Pool)
		want   bool
	}{
		{"snow of another color pays a colored shard", "MayPlaySnowIgnoreColor$ True", "G", func(p *engine.Pool) { p.AddSnow(mana.Red, 1) }, true},
		{"snow colorless pays a colored shard", "MayPlaySnowIgnoreColor$ True", "G", func(p *engine.Pool) { p.AddSnowColorless(1) }, true},
		{"plain mana of another color does not", "MayPlaySnowIgnoreColor$ True", "G", func(p *engine.Pool) { p.Add(mana.Red, 1) }, false},
		{"without the param it does not", "MayPlayWithFlash$ True", "G", func(p *engine.Pool) { p.AddSnow(mana.Red, 1) }, false},
		{"a {C} shard gets no relief", "MayPlaySnowIgnoreColor$ True", "C", func(p *engine.Pool) { p.AddSnow(mana.Green, 1) }, false},
		{"each colored shard takes its own snow mana", "MayPlaySnowIgnoreColor$ True", "R G", func(p *engine.Pool) {
			p.AddSnow(mana.White, 1)
			p.AddSnow(mana.Blue, 1)
		}, true},
		{"not more snow mana than shards", "MayPlaySnowIgnoreColor$ True", "R G", func(p *engine.Pool) { p.AddSnow(mana.White, 1) }, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			g, p, card := exileGrant(t, tc.params, tc.cost)
			tc.pool(&g.Player(p).ManaPool)
			if got := g.CastSpell(p, card, engine.NewScriptedController()); got != tc.want {
				t.Errorf("CastSpell = %v, want %v", got, tc.want)
			}
		})
	}
}

// TestMayPlaySnowIgnoreColorEndsWithTheCast proves the relaxation belongs to
// the cast that chose it: a later payment of the same player (here a plain
// spell from hand) pays colors as usual.
func TestMayPlaySnowIgnoreColorEndsWithTheCast(t *testing.T) {
	t.Parallel()

	g, p, card := exileGrant(t, "MayPlaySnowIgnoreColor$ True", "G")
	g.Player(p).ManaPool.AddSnow(mana.Red, 2)
	if !g.CastSpell(p, card, engine.NewScriptedController()) {
		t.Fatal("CastSpell through the snow grant = false")
	}
	if g.PayManaCost(p, mana.MustParse("G"), engine.NewScriptedController()) {
		t.Error("a later payment spent snow mana as the wrong color")
	}
}
