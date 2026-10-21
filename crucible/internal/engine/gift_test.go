package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
	"github.com/jczastkiewicz/crucible/internal/mana"
)

// Gift (CR 702.174): an optional cost that promises a gift to an opponent of
// the caster's choice. An instant or sorcery resolves its GiftAbility first
// (AbilityUtils.resolvePreAbilities), a permanent from a synthesized enters
// trigger; Defined$ Promised is the opponent.
func TestGiftSpellGivesTheGiftBeforeItsEffect(t *testing.T) {
	t.Parallel()

	for _, promise := range []bool{true, false} {
		g, p, other := newTwoPlayerGameOn(t, scenarioDB(t))
		g.SetTurnState(1, p, engine.Main1)
		for i := 0; i < 3; i++ {
			g.NewCard(creatureDefPT(t, "1", "1"), other, engine.Library)
		}
		before := len(g.Zone(engine.Hand, other).Cards())
		g.Player(p).Life = 20
		spell := g.NewCard(copyTestDef(t, "Test Gift Spell", "Instant", "", "", "Cost:G", "K:Gift",
			"A:SP$ GainLife | Defined$ You | LifeAmount$ 1",
			"SVar:GiftAbility:DB$ Draw | Defined$ Promised | NumCards$ 1 | GiftDescription$ a card"), p, engine.Hand)
		g.Player(p).ManaPool.Add(mana.Green, 1)
		c := engine.NewScriptedController()
		c.QueueConfirmPayCost(promise)
		if promise {
			c.QueuePlayerChoice(other)
		}
		if !g.CastSpell(p, spell, c) {
			t.Fatalf("promise=%v: cast failed: %v", promise, g.TakePendingError())
		}
		if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
			t.Fatalf("promise=%v: resolve: %v", promise, err)
		}
		drawn := len(g.Zone(engine.Hand, other).Cards()) - before
		want := 0
		if promise {
			want = 1
		}
		if drawn != want {
			t.Errorf("promise=%v: opponent drew %d, want %d", promise, drawn, want)
		}
		if got := g.Player(p).Life; got != 21 {
			t.Errorf("promise=%v: life = %d, want 21 (the spell's own effect still happens)", promise, got)
		}
	}
}

// A permanent's gift is given when it enters, only if it was promised.
func TestGiftPermanentGivesTheGiftWhenItEnters(t *testing.T) {
	t.Parallel()

	for _, promise := range []bool{true, false} {
		g, p, other := newTwoPlayerGameOn(t, scenarioDB(t))
		g.SetTurnState(1, p, engine.Main1)
		for i := 0; i < 3; i++ {
			g.NewCard(creatureDefPT(t, "1", "1"), other, engine.Library)
		}
		before := len(g.Zone(engine.Hand, other).Cards())
		body := g.NewCard(copyTestDef(t, "Test Gift Body", "Creature", "1", "1", "Cost:G", "K:Gift",
			"SVar:GiftAbility:DB$ Draw | Defined$ Promised | NumCards$ 1 | GiftDescription$ a card"), p, engine.Hand)
		g.Player(p).ManaPool.Add(mana.Green, 1)
		c := engine.NewScriptedController()
		c.QueueConfirmPayCost(promise)
		if promise {
			c.QueuePlayerChoice(other)
		}
		if !g.CastSpell(p, body, c) {
			t.Fatalf("promise=%v: cast failed", promise)
		}
		if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
			t.Fatalf("promise=%v: resolve: %v", promise, err)
		}
		drawn := len(g.Zone(engine.Hand, other).Cards()) - before
		want := 0
		if promise {
			want = 1
		}
		if drawn != want {
			t.Errorf("promise=%v: opponent drew %d, want %d", promise, drawn, want)
		}
	}
}
