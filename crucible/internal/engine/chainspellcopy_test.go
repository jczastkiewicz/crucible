package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
	"github.com/jczastkiewicz/crucible/internal/mana"
)

// TestChainLightningCopiesItselfWhenTheTargetPays proves Defined$ Parent
// (AbilityUtils.java:1232-1233) with UnlessSwitched$: the targeted player
// pays {R}{R}, becomes the copier, and the copy of the root spell hits the
// target they chose; declining leaves one hit and no copy.
func TestChainLightningCopiesItselfWhenTheTargetPays(t *testing.T) {
	t.Parallel()

	for _, pay := range []bool{true, false} {
		g, p, other := newTwoPlayerGameOn(t, scenarioDB(t))
		bolt := g.NewCard(corpusCard(t, "Chain Lightning"), p, engine.Hand)
		g.Player(p).ManaPool.Add(mana.Red, 1)
		g.Player(other).ManaPool.Add(mana.Red, 2)
		start := g.Player(p).Life

		c := engine.NewScriptedController()
		c.QueueTargets([]engine.EntityID{engine.PlayerEntity(other)})
		c.QueueConfirmPayCost(pay)
		if pay {
			// The copier keeps no targets, picks the caster, and the caster
			// then declines to pay for the copy's own chain link.
			c.QueueConfirmEffect(true)
			c.QueueTargets([]engine.EntityID{engine.PlayerEntity(p)})
			c.QueueConfirmPayCost(false)
		}
		if !g.CastSpell(p, bolt, c) {
			t.Fatalf("CastSpell failed: %v", g.TakePendingError())
		}
		if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
			t.Fatalf("pay=%v: ResolveStack: %v", pay, err)
		}
		if got := g.Player(other).Life; got != 17 {
			t.Errorf("pay=%v: original target life = %d, want 17", pay, got)
		}
		wantCaster := start
		if pay {
			wantCaster = start - 3
		}
		if got := g.Player(p).Life; got != wantCaster {
			t.Errorf("pay=%v: caster life = %d, want %d", pay, got, wantCaster)
		}
	}
}

// TestChooseTypeAtRandomBasicLandPicksOneOfTheFive proves ChooseType AtRandom$
// over Type$ Basic Land (ChooseTypeEffect.java:68-70, :129-131): the host's
// chosen type is one basic land type, with no controller decision.
func TestChooseTypeAtRandomBasicLandPicksOneOfTheFive(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGameOn(t, scenarioDB(t))
	def := etbChainDef(t, "Test Random Basic", "DB$ ChooseType | Defined$ You | Type$ Basic Land | AtRandom$ True")
	host, err := castETBChain(t, g, p, def, engine.NewScriptedController())
	if err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	got := g.Card(host).Memory.ChosenType(false)
	for _, want := range []string{"Mountain", "Swamp", "Forest", "Plains", "Island"} {
		if got == want {
			return
		}
	}
	t.Errorf("chosen type = %q, want a basic land type", got)
}
