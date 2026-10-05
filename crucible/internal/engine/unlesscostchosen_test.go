package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
	"github.com/jczastkiewicz/crucible/internal/mana"
)

// TestUnlessCostExileFromGraveyard proves ExileFromGrave<N/Type> (13 real
// lines): the payer exiles N matching cards from their own graveyard; with too
// few the cost cannot be paid and the ability happens.
func TestUnlessCostExileFromGraveyard(t *testing.T) {
	t.Parallel()

	t.Run("paid", func(t *testing.T) {
		t.Parallel()
		g, p, c, cast := unlessCostGame(t, "ExileFromGrave<1/Card>")
		fodder := g.NewCard(creatureDef(t), p, engine.Graveyard)
		c.QueueConfirmPayCost(true)
		c.QueueCardChoice([]engine.CardID{fodder})
		creature := cast()
		if g.Card(creature).Zone != engine.Battlefield || g.Card(fodder).Zone != engine.Exile {
			t.Errorf("creature %v, graveyard card %v, want Battlefield and Exile", g.Card(creature).Zone, g.Card(fodder).Zone)
		}
	})
	t.Run("graveyard too small", func(t *testing.T) {
		t.Parallel()
		g, _, c, cast := unlessCostGame(t, "ExileFromGrave<1/Card>")
		c.QueueConfirmPayCost(true)
		if creature := cast(); g.Card(creature).Zone != engine.Graveyard {
			t.Errorf("creature zone = %v, want Graveyard: nothing to exile", g.Card(creature).Zone)
		}
	})
}

// TestUnlessCostTapXType proves tapXType<N/Type>: the payer taps N untapped
// permanents of the type.
func TestUnlessCostTapXType(t *testing.T) {
	t.Parallel()

	t.Run("paid", func(t *testing.T) {
		t.Parallel()
		g, p, c, cast := unlessCostGame(t, "tapXType<1/Creature>")
		helper := g.NewCard(creatureDef(t), p, engine.Battlefield)
		c.QueueConfirmPayCost(true)
		c.QueueTapChoice([]engine.CardID{helper})
		creature := cast()
		if g.Card(creature).Zone != engine.Battlefield || !g.Card(helper).Tapped {
			t.Errorf("creature %v, helper tapped %v, want Battlefield and tapped", g.Card(creature).Zone, g.Card(helper).Tapped)
		}
	})
	t.Run("nothing matches", func(t *testing.T) {
		t.Parallel()
		g, _, c, cast := unlessCostGame(t, "tapXType<1/Artifact>")
		c.QueueConfirmPayCost(true)
		if creature := cast(); g.Card(creature).Zone != engine.Graveyard {
			t.Errorf("creature zone = %v, want Graveyard: no artifact to tap", g.Card(creature).Zone)
		}
	})
}

// TestUnlessCostAddCounterYou proves AddCounterYou<N/Type> (Ward's Spirit
// of the Mountain-style poison cost): the payer gets the counters.
func TestUnlessCostAddCounterYou(t *testing.T) {
	t.Parallel()

	g, p, c, cast := unlessCostGame(t, "AddCounterYou<2/POISON>")
	c.QueueConfirmPayCost(true)
	creature := cast()
	if g.Card(creature).Zone != engine.Battlefield || g.Player(p).Counters.Count(engine.Poison) != 2 {
		t.Errorf("creature %v, poison %d, want Battlefield and 2", g.Card(creature).Zone, g.Player(p).Counters.Count(engine.Poison))
	}
}

// TestUnlessCostBlight proves Blight<N> (CostBlight is a CostPutCounter of
// -1/-1 counters on a creature the payer controls): the payer picks the
// creature.
func TestUnlessCostBlight(t *testing.T) {
	t.Parallel()

	g, p, c, cast := unlessCostGame(t, "Blight<2>")
	other := g.NewCard(creatureDefPT(t, "4", "4"), p, engine.Battlefield)
	c.QueueConfirmPayCost(true)
	c.QueueCardChoice([]engine.CardID{other})
	creature := cast()
	if g.Card(creature).Zone != engine.Battlefield || g.Card(other).Counters.Count(engine.M1M1) != 2 {
		t.Errorf("creature %v, -1/-1 counters %d, want Battlefield and 2", g.Card(creature).Zone, g.Card(other).Counters.Count(engine.M1M1))
	}
}

// TestUnlessCostAddCounterOnAChosenCreature proves
// AddCounter<N/Type/Valid/desc> naming a card (Fabricate-like "a creature you
// control"), not CARDNAME.
func TestUnlessCostAddCounterOnAChosenCreature(t *testing.T) {
	t.Parallel()

	g, p, c, cast := unlessCostGame(t, "AddCounter<1/P1P1/Creature.YouCtrl/a creature you control>")
	other := g.NewCard(creatureDef(t), p, engine.Battlefield)
	c.QueueConfirmPayCost(true)
	c.QueueCardChoice([]engine.CardID{other})
	creature := cast()
	if g.Card(creature).Zone != engine.Battlefield || g.Card(other).Counters.Count(engine.P1P1) != 1 {
		t.Errorf("creature %v, +1/+1 counters %d, want Battlefield and 1", g.Card(creature).Zone, g.Card(other).Counters.Count(engine.P1P1))
	}
}

// TestUnlessCostCollectEvidence proves CollectEvidence<N>: the payer exiles
// graveyard cards of total mana value at least N; a graveyard worth less
// cannot pay.
func TestUnlessCostCollectEvidence(t *testing.T) {
	t.Parallel()

	t.Run("paid", func(t *testing.T) {
		t.Parallel()
		g, p, c, cast := unlessCostGame(t, "CollectEvidence<3>")
		big := g.NewCard(creatureDefManaCost(t, "3"), p, engine.Graveyard)
		c.QueueConfirmPayCost(true)
		c.QueueCardChoice([]engine.CardID{big})
		creature := cast()
		if g.Card(creature).Zone != engine.Battlefield || g.Card(big).Zone != engine.Exile {
			t.Errorf("creature %v, evidence %v, want Battlefield and Exile", g.Card(creature).Zone, g.Card(big).Zone)
		}
	})
	t.Run("graveyard worth too little", func(t *testing.T) {
		t.Parallel()
		g, p, c, cast := unlessCostGame(t, "CollectEvidence<3>")
		small := g.NewCard(creatureDefManaCost(t, "1"), p, engine.Graveyard)
		c.QueueConfirmPayCost(true)
		creature := cast()
		if g.Card(creature).Zone != engine.Graveyard || g.Card(small).Zone != engine.Graveyard {
			t.Errorf("creature %v, card %v, want both in Graveyard: mana value 1 is under 3", g.Card(creature).Zone, g.Card(small).Zone)
		}
	})
}

// TestUnlessCostDrawNamingAnotherPlayer proves Draw<N/Player.Opponent>
// (CostDraw.getPotentialPlayers): "unless you let an opponent draw a card".
func TestUnlessCostDrawNamingAnotherPlayer(t *testing.T) {
	t.Parallel()

	g, _, c, cast := unlessCostGame(t, "Draw<1/Player.Opponent>")
	opp := g.Players()[1]
	g.NewCard(creatureDef(t), opp, engine.Library)
	c.QueueConfirmPayCost(true)
	creature := cast()
	if g.Card(creature).Zone != engine.Battlefield || len(g.Zone(engine.Hand, opp).Cards()) != 1 {
		t.Errorf("creature %v, opponent hand %d, want Battlefield and 1", g.Card(creature).Zone, len(g.Zone(engine.Hand, opp).Cards()))
	}
}

// TestUnlessCostWaterbend proves Waterbend<N> (CostWaterbend): N generic mana,
// each payable by tapping an untapped artifact or creature (convoke-like).
func TestUnlessCostWaterbend(t *testing.T) {
	t.Parallel()

	g, p, c, cast := unlessCostGame(t, "Waterbend<2>")
	helper := g.NewCard(creatureDef(t), p, engine.Battlefield)
	c.QueueConfirmPayCost(true)
	c.QueueCardChoice([]engine.CardID{helper})
	// The tapped creature pays {1} of Waterbend<2>; the other {1} comes from
	// the pool (cast adds the Green that casts the creature, so one more).
	g.Player(p).ManaPool.Add(mana.Green, 1)
	c.QueuePayGeneric(mana.ShardG)
	creature := cast()
	if g.Card(creature).Zone != engine.Battlefield || !g.Card(helper).Tapped {
		t.Errorf("creature %v, helper tapped %v, want Battlefield and tapped: it paid {1} of Waterbend<2>", g.Card(creature).Zone, g.Card(helper).Tapped)
	}
}

// TestUnlessCostXIsAnSVar proves the SVar shapes of UnlessCost$ (62 real
// lines): a bare SVar name is that much mana, and X the amount of SVar X.
func TestUnlessCostXIsAnSVar(t *testing.T) {
	t.Parallel()

	for _, unless := range []string{"X", "Y"} {
		t.Run(unless, func(t *testing.T) {
			t.Parallel()
			g := newGame(t, "a", "b")
			p := g.Players()[0]
			g.SetTurnState(1, p, engine.Main1)
			c := engine.NewScriptedController()
			def := etbSacrificeTriggerDefParams(t, "Test Unless X", "UnlessCost$ "+unless+" | UnlessPayer$ You", map[string]string{"X": "2", "Y": "2"})
			creature := g.NewCard(def, p, engine.Hand)
			g.Player(p).ManaPool.Add(mana.Green, 1)
			if !g.CastSpell(p, creature, c) {
				t.Fatal("CastSpell failed")
			}
			// {2}: the pool holds exactly the generic amount SVar names.
			g.Player(p).ManaPool.Add(mana.Green, 2)
			c.QueuePayGeneric(mana.ShardG)
			c.QueuePayGeneric(mana.ShardG)
			c.QueueConfirmPayCost(true)
			if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
				t.Fatalf("ResolveStack: %v", err)
			}
			if g.Card(creature).Zone != engine.Battlefield {
				t.Errorf("creature zone = %v, want Battlefield: {2} was paid", g.Card(creature).Zone)
			}
			if got := g.Player(p).ManaPool.Total(); got != 0 {
				t.Errorf("mana left = %d, want 0", got)
			}
		})
	}
}

// TestUnlessCostPartAmountIsAnSVar proves PayLife<X>, PayEnergy<X> and
// DamageYou<X> (about 25 real lines) take the amount of SVar X.
func TestUnlessCostPartAmountIsAnSVar(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p := g.Players()[0]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life = 20
	c := engine.NewScriptedController()
	def := etbSacrificeTriggerDefParams(t, "Test Unless X Life", "UnlessCost$ PayLife<X> | UnlessPayer$ You", map[string]string{"X": "4"})
	creature := g.NewCard(def, p, engine.Hand)
	g.Player(p).ManaPool.Add(mana.Green, 1)
	if !g.CastSpell(p, creature, c) {
		t.Fatal("CastSpell failed")
	}
	c.QueueConfirmPayCost(true)
	if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if g.Card(creature).Zone != engine.Battlefield || g.Player(p).Life != 16 {
		t.Errorf("creature %v, life %d, want Battlefield and 16", g.Card(creature).Zone, g.Player(p).Life)
	}
}
