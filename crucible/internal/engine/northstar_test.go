package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
	"github.com/jczastkiewicz/crucible/internal/mana"
)

// North Star: "For one spell this turn, you may spend mana as though it were
// mana of any type to pay that spell's mana cost." The Effect's ManaConvert
// carries Optional$, asked as the spell's cost is paid; a yes remembers the
// spell, and ForgetOnCast$ Card.IsRemembered ends the Effect once it is cast
// (StaticAbilityManaConvert.java:41-47, EffectEffect.java:253-254).
func TestNorthStarPaysOneSpellWithAnyTypeOfMana(t *testing.T) {
	t.Parallel()

	for name, yes := range map[string]bool{"accepted": true, "declined": false} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			g, p, other := newTwoPlayerGameOn(t, scenarioDB(t))
			g.SetTurnState(1, p, engine.Main1)
			star := g.NewCard(corpusCard(t, "North Star"), p, engine.Battlefield)
			g.Player(p).ManaPool.Add(mana.Green, 4)
			c := engine.NewScriptedController()
			for range 4 {
				c.QueuePayGeneric(mana.ShardG)
			}
			if !g.ActivateAbility(p, star, 0, c) {
				t.Fatalf("North Star did not activate: %v", g.TakePendingError())
			}
			if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
				t.Fatal(err)
			}

			bolt := g.NewCard(corpusCard(t, "Lightning Bolt"), p, engine.Hand)
			g.Player(p).ManaPool.Add(mana.Green, 1)
			c.QueueConfirmEffect(yes)
			if yes {
				c.QueuePayGeneric(mana.ShardG)
			}
			c.QueueTargets([]engine.EntityID{engine.PlayerEntity(other)})
			cast := g.CastSpell(p, bolt, c)
			if cast != yes {
				t.Fatalf("cast = %v, want %v (%v)", cast, yes, g.TakePendingError())
			}
			if !yes {
				return
			}
			if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
				t.Fatal(err)
			}
			if got := g.Player(other).Life; got != 17 {
				t.Errorf("opponent life = %d, want 17", got)
			}
			for _, id := range g.Zone(engine.Command, p).Cards() {
				if g.Card(id).IsEffect {
					t.Errorf("North Star's effect is still in the command zone after the spell")
				}
			}
		})
	}
}
