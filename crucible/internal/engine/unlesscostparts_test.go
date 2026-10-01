package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
	"github.com/jczastkiewicz/crucible/internal/mana"
)

// unlessCostGame is a game with the ETB-sacrifice creature of unlesscost_test.go
// ("sacrifice CARDNAME unless you pay ...") for the given UnlessCost$ text:
// paying prevents the sacrifice, not paying lets it happen.
func unlessCostGame(t *testing.T, unless string) (g *engine.Game, p engine.PlayerID, c *engine.ScriptedController, cast func() engine.CardID) {
	t.Helper()
	g = newGame(t, "a", "b")
	p = g.Players()[0]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life, g.Player(g.Players()[1]).Life = 20, 20
	c = engine.NewScriptedController()
	def := etbSacrificeTriggerDefParams(t, "Test Unless Part", "UnlessCost$ "+unless+" | UnlessPayer$ You", nil)
	cast = func() engine.CardID {
		creature, err := castETBSacrifice(t, g, p, def, c)
		if err != nil {
			t.Fatalf("ResolveStack: %v", err)
		}
		return creature
	}
	return g, p, c, cast
}

// TestUnlessCostPayLife proves a PayLife<N> UnlessCost$ (68 real lines): paying
// takes the life and prevents the ability; a life total below N cannot pay.
func TestUnlessCostPayLife(t *testing.T) {
	t.Parallel()

	t.Run("paid", func(t *testing.T) {
		t.Parallel()
		g, p, c, cast := unlessCostGame(t, "PayLife<3>")
		c.QueueConfirmPayCost(true)
		creature := cast()
		if g.Card(creature).Zone != engine.Battlefield {
			t.Errorf("creature zone = %v, want Battlefield: the cost was paid", g.Card(creature).Zone)
		}
		if got := g.Player(p).Life; got != 17 {
			t.Errorf("life = %d, want 17", got)
		}
	})
	t.Run("declined", func(t *testing.T) {
		t.Parallel()
		g, p, c, cast := unlessCostGame(t, "PayLife<3>")
		c.QueueConfirmPayCost(false)
		creature := cast()
		if g.Card(creature).Zone != engine.Graveyard || g.Player(p).Life != 20 {
			t.Errorf("zone %v life %d, want Graveyard and 20", g.Card(creature).Zone, g.Player(p).Life)
		}
	})
	t.Run("cannot afford", func(t *testing.T) {
		t.Parallel()
		g, p, c, cast := unlessCostGame(t, "PayLife<3>")
		g.Player(p).Life = 2
		c.QueueConfirmPayCost(true)
		creature := cast()
		if g.Card(creature).Zone != engine.Graveyard || g.Player(p).Life != 2 {
			t.Errorf("zone %v life %d, want Graveyard and 2: life 2 cannot pay 3", g.Card(creature).Zone, g.Player(p).Life)
		}
	})
}

// TestUnlessCostDiscard proves Discard<N/Card> (83 real lines): the payer
// chooses the discards, and with too few cards in hand cannot pay.
func TestUnlessCostDiscard(t *testing.T) {
	t.Parallel()

	t.Run("paid", func(t *testing.T) {
		t.Parallel()
		g, p, c, cast := unlessCostGame(t, "Discard<1/Card>")
		held := g.NewCard(creatureDef(t), p, engine.Hand)
		c.QueueConfirmPayCost(true)
		c.QueueDiscardChoice([]engine.CardID{held})
		creature := cast()
		if g.Card(creature).Zone != engine.Battlefield || g.Card(held).Zone != engine.Graveyard {
			t.Errorf("creature %v, discarded card %v, want Battlefield and Graveyard", g.Card(creature).Zone, g.Card(held).Zone)
		}
	})
	t.Run("empty hand", func(t *testing.T) {
		t.Parallel()
		g, _, c, cast := unlessCostGame(t, "Discard<1/Card>")
		c.QueueConfirmPayCost(true)
		if creature := cast(); g.Card(creature).Zone != engine.Graveyard {
			t.Errorf("creature zone = %v, want Graveyard: nothing to discard", g.Card(creature).Zone)
		}
	})
}

// TestUnlessCostSacrificesAChosenPermanent proves Sac<N/Type> (88 real
// lines): the payer sacrifices a matching permanent they choose.
func TestUnlessCostSacrificesAChosenPermanent(t *testing.T) {
	t.Parallel()

	t.Run("paid", func(t *testing.T) {
		t.Parallel()
		g, p, c, cast := unlessCostGame(t, "Sac<1/Creature>")
		fodder := g.NewCard(creatureDef(t), p, engine.Battlefield)
		c.QueueConfirmPayCost(true)
		c.QueueSacrificeChoice([]engine.CardID{fodder})
		creature := cast()
		if g.Card(creature).Zone != engine.Battlefield || g.Card(fodder).Zone != engine.Graveyard {
			t.Errorf("creature %v, fodder %v, want Battlefield and Graveyard", g.Card(creature).Zone, g.Card(fodder).Zone)
		}
	})
	t.Run("nothing matches", func(t *testing.T) {
		t.Parallel()
		g, p, c, cast := unlessCostGame(t, "Sac<1/Land>")
		g.NewCard(creatureDef(t), p, engine.Battlefield)
		c.QueueConfirmPayCost(true)
		if creature := cast(); g.Card(creature).Zone != engine.Graveyard {
			t.Errorf("creature zone = %v, want Graveyard: no land to sacrifice", g.Card(creature).Zone)
		}
	})
}

// TestUnlessCostMixedPartsPayAtomically proves "{G} PayLife<2>" pays both or
// neither: with no mana to pay, the life stays too.
func TestUnlessCostMixedPartsPayAtomically(t *testing.T) {
	t.Parallel()

	t.Run("both", func(t *testing.T) {
		t.Parallel()
		g, p, c, cast := unlessCostGame(t, "G PayLife<2>")
		g.Player(p).ManaPool.Add(mana.Green, 1)
		c.QueueConfirmPayCost(true)
		creature := cast()
		if g.Card(creature).Zone != engine.Battlefield || g.Player(p).Life != 18 {
			t.Errorf("zone %v life %d, want Battlefield and 18", g.Card(creature).Zone, g.Player(p).Life)
		}
	})
	t.Run("no mana", func(t *testing.T) {
		t.Parallel()
		g, p, c, cast := unlessCostGame(t, "G PayLife<2>")
		c.QueueConfirmPayCost(true)
		creature := cast()
		if g.Card(creature).Zone != engine.Graveyard || g.Player(p).Life != 20 {
			t.Errorf("zone %v life %d, want Graveyard and 20: a failed payment pays nothing", g.Card(creature).Zone, g.Player(p).Life)
		}
	})
}

// TestWardPayLifeCountersUnlessPaid proves Ward:PayLife<2> end to end
// (21 real lines): the targeting spell's controller pays 2 life to keep the
// spell, or the spell is countered.
func TestWardPayLifeCountersUnlessPaid(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name       string
		pay        bool
		wantLife   int
		wantTarget engine.ZoneType
	}{
		{"paid", true, 18, engine.Graveyard},
		{"declined", false, 20, engine.Battlefield},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			g := newGame(t, "a", "b")
			p, opp := g.Players()[0], g.Players()[1]
			g.Player(p).Life, g.Player(opp).Life = 20, 20
			target := g.NewCard(creatureDefPTKeywords(t, "3", "1", "Ward:PayLife<2>"), p, engine.Battlefield)
			spell := g.NewCard(instantDefWithAbility(t, "Test Bolt", "0", "SP$ DealDamage | ValidTgts$ Creature | NumDmg$ 3"), opp, engine.Hand)
			g.SetTurnState(1, p, engine.Main1)

			c := engine.NewScriptedController()
			c.QueueTargets([]engine.EntityID{engine.CardEntity(target)})
			if !g.CastSpell(opp, spell, c) {
				t.Fatal("CastSpell failed")
			}
			c.QueueConfirmPayCost(tc.pay)
			if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
				t.Fatalf("ResolveStack: %v", err)
			}
			if got := g.Player(opp).Life; got != tc.wantLife {
				t.Errorf("payer life = %d, want %d", got, tc.wantLife)
			}
			if got := g.Card(target).Zone; got != tc.wantTarget {
				t.Errorf("warded creature zone = %v, want %v (the bolt resolves only when Ward's cost was paid)", got, tc.wantTarget)
			}
		})
	}
}

// TestWardDiscardCountersUnlessPaid is the same for Ward:Discard<1/Card>
// (14 real lines).
func TestWardDiscardCountersUnlessPaid(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p, opp := g.Players()[0], g.Players()[1]
	g.Player(p).Life, g.Player(opp).Life = 20, 20
	target := g.NewCard(creatureDefPTKeywords(t, "3", "1", "Ward:Discard<1/Card>"), p, engine.Battlefield)
	spell := g.NewCard(instantDefWithAbility(t, "Test Bolt", "0", "SP$ DealDamage | ValidTgts$ Creature | NumDmg$ 3"), opp, engine.Hand)
	held := g.NewCard(creatureDef(t), opp, engine.Hand)
	g.SetTurnState(1, p, engine.Main1)

	c := engine.NewScriptedController()
	c.QueueTargets([]engine.EntityID{engine.CardEntity(target)})
	if !g.CastSpell(opp, spell, c) {
		t.Fatal("CastSpell failed")
	}
	c.QueueConfirmPayCost(true)
	c.QueueDiscardChoice([]engine.CardID{held})
	if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if g.Card(held).Zone != engine.Graveyard || g.Card(target).Zone != engine.Graveyard {
		t.Errorf("discarded card %v, warded creature %v, want both Graveyard (the cost was paid, the bolt resolved)",
			g.Card(held).Zone, g.Card(target).Zone)
	}
}
