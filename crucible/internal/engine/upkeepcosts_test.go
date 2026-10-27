package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
	"github.com/jczastkiewicz/crucible/internal/mana"
)

// toStep walks g to the next step of phase for pid and runs before once it has
// begun (floating mana empties between steps, so it has to arrive after), then
// resolves the stack. It resolves the stack at every earlier step.
func toStep(t *testing.T, g *engine.Game, c engine.PlayerController, pid engine.PlayerID, phase engine.PhaseType, before func()) {
	t.Helper()
	for i := 0; i < 120; i++ {
		g.AdvancePhase(c)
		if g.ActivePhase() == phase && g.ActivePlayer() == pid {
			if before != nil {
				before()
			}
			if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
				t.Fatalf("ResolveStack at %v: %v", phase, err)
			}
			return
		}
		if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
			t.Fatalf("ResolveStack at %v: %v", g.ActivePhase(), err)
		}
	}
	t.Fatalf("never reached %v", phase)
}

// toUpkeep walks g to pid's next upkeep and hands them mana before the triggers
// that step pushed resolve.
func toUpkeep(t *testing.T, g *engine.Game, c engine.PlayerController, pid engine.PlayerID, color mana.Colors, n int) {
	t.Helper()
	toStep(t, g, c, pid, engine.Upkeep, func() { g.Player(pid).ManaPool.Add(color, n) })
}

// Echo (CR 702.30a): at the beginning of the first upkeep after it came under
// your control, sacrifice it unless you pay its echo cost; the next upkeeps
// ask nothing.
func TestEchoIsPaidOnceOrTheCreatureIsSacrificed(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name string
		pay  bool
		want engine.ZoneType
	}{
		{"paid", true, engine.Battlefield},
		{"unpaid", false, engine.Graveyard},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			g, p, other := newTwoPlayerGameOn(t, scenarioDB(t))
			libraryCards(t, g, p, 8)
			libraryCards(t, g, other, 8)
			buggy := g.NewCard(corpusCard(t, "Goblin War Buggy"), p, engine.Battlefield)
			g.SetTurnState(2, p, engine.Untap)
			c := engine.NewScriptedController()
			c.QueueConfirmPayCost(tc.pay)
			if tc.pay {
				c.QueuePayGeneric(mana.ShardR)
			}
			toUpkeep(t, g, c, p, mana.Red, 2)
			if got := g.Card(buggy).Zone; got != tc.want {
				t.Fatalf("after the first upkeep the Buggy is in %v, want %v", got, tc.want)
			}
			if !tc.pay {
				return
			}
			// The next upkeep of the same player asks nothing: no decision is
			// queued for it, and the controller declines anything unqueued.
			toUpkeep(t, g, c, p, mana.Red, 2)
			if got := g.Card(buggy).Zone; got != engine.Battlefield {
				t.Errorf("on its second upkeep the Buggy is in %v, want Battlefield", got)
			}
		})
	}
}

// Cumulative upkeep (CR 702.24a): each upkeep an age counter goes on, and the
// cost is paid once per counter; declining sacrifices it.
func TestCumulativeUpkeepCostsMoreEachTurn(t *testing.T) {
	t.Parallel()

	g, p, other := newTwoPlayerGameOn(t, scenarioDB(t))
	libraryCards(t, g, p, 8)
	libraryCards(t, g, other, 8)
	fallen := g.NewCard(corpusCard(t, "Balduvian Fallen"), p, engine.Battlefield)
	g.SetTurnState(2, p, engine.Untap)
	c := engine.NewScriptedController()

	c.QueueConfirmPayCost(true)
	c.QueuePayGeneric(mana.ShardB)
	toUpkeep(t, g, c, p, mana.Black, 3)
	if got := g.Card(fallen).Counters.Count(engine.Age); got != 1 {
		t.Fatalf("age counters after the first upkeep = %d, want 1", got)
	}
	if got := g.Player(p).ManaPool.Total(); got != 2 {
		t.Errorf("mana left = %d, want 2 after paying {1} once", got)
	}

	c.QueueConfirmPayCost(true)
	c.QueuePayGeneric(mana.ShardB)
	c.QueuePayGeneric(mana.ShardB)
	toUpkeep(t, g, c, p, mana.Black, 3)
	if got := g.Card(fallen).Counters.Count(engine.Age); got != 2 {
		t.Fatalf("age counters after the second upkeep = %d, want 2", got)
	}
	if got := g.Player(p).ManaPool.Total(); got != 1 {
		t.Errorf("mana left = %d, want 1 after paying {1}{1}", got)
	}

	c.QueueConfirmPayCost(false)
	toUpkeep(t, g, c, p, mana.Black, 3)
	if got := g.Card(fallen).Zone; got != engine.Graveyard {
		t.Errorf("after declining the third payment the creature is in %v, want Graveyard", got)
	}
}

// Cumulative upkeep parts past mana: Thought Lash exiles cards off the top of
// its controller's library, Wall of Shards gives an opponent life, Braid of
// Fire adds mana (which fuels the next upkeep's spells), each once per age
// counter.
func TestCumulativeUpkeepLibraryLifeAndManaParts(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		card  string
		check func(t *testing.T, g *engine.Game, p, other engine.PlayerID, libBefore int)
	}{
		{"Thought Lash", func(t *testing.T, g *engine.Game, p, _ engine.PlayerID, libBefore int) {
			if got := g.Zone(engine.Library, p).Len(); got >= libBefore {
				t.Errorf("library %d -> %d, want cards exiled off the top", libBefore, got)
			}
		}},
		{"Wall of Shards", func(t *testing.T, g *engine.Game, _, other engine.PlayerID, _ int) {
			if got := g.Player(other).Life; got != 21 {
				t.Errorf("opponent life = %d, want 21", got)
			}
		}},
		{"Braid of Fire", func(t *testing.T, g *engine.Game, p, _ engine.PlayerID, _ int) {
			if got := g.Player(p).ManaPool.Total(); got != 1 {
				t.Errorf("mana in the pool = %d, want the {R} the cost added", got)
			}
		}},
	} {
		t.Run(tc.card, func(t *testing.T) {
			t.Parallel()

			g, p, other := newTwoPlayerGameOn(t, scenarioDB(t))
			libraryCards(t, g, p, 8)
			libraryCards(t, g, other, 8)
			card := g.NewCard(corpusCard(t, tc.card), p, engine.Battlefield)
			g.SetTurnState(2, p, engine.Untap)
			c := engine.NewScriptedController()
			c.QueueConfirmPayCost(true)
			libBefore := g.Zone(engine.Library, p).Len()
			toStep(t, g, c, p, engine.Upkeep, nil)
			if got := g.Card(card).Zone; got != engine.Battlefield {
				t.Fatalf("%s is in %v after paying, want the battlefield", tc.card, got)
			}
			tc.check(t, g, p, other, libBefore)
		})
	}
}

// Herald of Leshrac's cumulative upkeep is GainControl<1/Land.YouDontCtrl>: the
// payer picks a land they do not control and gains control of it
// (CostGainControl); unpaid, the Herald is sacrificed.
func TestCumulativeUpkeepGainControlTakesALand(t *testing.T) {
	t.Parallel()

	for _, pay := range []bool{true, false} {
		g, p, other := newTwoPlayerGameOn(t, scenarioDB(t))
		libraryCards(t, g, p, 8)
		libraryCards(t, g, other, 8)
		herald := g.NewCard(corpusCard(t, "Herald of Leshrac"), p, engine.Battlefield)
		land := g.NewCard(corpusCard(t, "Swamp"), other, engine.Battlefield)
		g.SetTurnState(2, p, engine.Untap)
		c := engine.NewScriptedController()
		c.QueueConfirmPayCost(pay)
		c.QueueCardChoice([]engine.CardID{land})
		toStep(t, g, c, p, engine.Upkeep, nil)
		heraldZone, controller := g.Card(herald).Zone, g.Card(land).Controller()
		if pay && (heraldZone != engine.Battlefield || controller != p) {
			t.Errorf("paid: Herald in %v, land controlled by %v, want the battlefield and %v", heraldZone, controller, p)
		}
		if !pay && (heraldZone != engine.Graveyard || controller != other) {
			t.Errorf("unpaid: Herald in %v, land controlled by %v, want the graveyard and %v", heraldZone, controller, other)
		}
	}
}

// Jötun Grunt's cumulative upkeep is PutCardToLibFromSameGrave<2/-1/Card>: two
// cards of one graveyard go to the bottom of their owner's library.
func TestCumulativeUpkeepPutsTwoCardsFromOneGraveyardOnTheBottom(t *testing.T) {
	t.Parallel()

	g, p, other := newTwoPlayerGameOn(t, scenarioDB(t))
	libraryCards(t, g, p, 8)
	libraryCards(t, g, other, 8)
	g.NewCard(corpusCard(t, "Jötun Grunt"), p, engine.Battlefield)
	a := g.NewCard(creatureDefPT(t, "1", "1"), other, engine.Graveyard)
	b := g.NewCard(creatureDefPT(t, "1", "1"), other, engine.Graveyard)
	g.NewCard(creatureDefPT(t, "1", "1"), p, engine.Graveyard)
	g.SetTurnState(2, p, engine.Untap)
	c := engine.NewScriptedController()
	c.QueueConfirmPayCost(true)
	c.QueuePlayerChoice(other)
	c.QueueCardChoice([]engine.CardID{a, b})
	toStep(t, g, c, p, engine.Upkeep, nil)
	lib := g.Zone(engine.Library, other).Cards()
	if len(lib) != 10 || lib[len(lib)-2] != a && lib[len(lib)-1] != a || lib[len(lib)-2] != b && lib[len(lib)-1] != b {
		t.Errorf("opponent library = %v, want %v and %v on the bottom", lib, a, b)
	}
	if n := g.Zone(engine.Graveyard, p).Len(); n != 1 {
		t.Errorf("own graveyard holds %d cards, want it untouched", n)
	}
}

// XColor$ restricts the colors the announced X is paid with (Drain Life):
// red mana cannot pay it, black mana can (SpellAbility.getXColor).
func TestXColorLimitsTheManaThatPaysX(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name string
		pool mana.Colors
		want int
	}{
		{"black pays", mana.Black, 22},
		{"red cannot", mana.Red, 20},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			g, p, _ := newTwoPlayerGameOn(t, scenarioDB(t))
			spell := g.NewCard(copyTestDef(t, "Test Drain", "Sorcery", "", "", "Cost:X",
				"A:SP$ GainLife | Cost$ X | XColor$ Black | Defined$ You | LifeAmount$ 2"), p, engine.Hand)
			g.Player(p).ManaPool.Add(tc.pool, 2)
			c := engine.NewScriptedController()
			c.QueuePayX(2)
			cast := g.CastSpell(p, spell, c)
			if cast != (tc.want == 22) {
				t.Fatalf("CastSpell = %v, want success only for black", cast)
			}
			if cast {
				if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
					t.Fatal(err)
				}
			}
			if got := g.Player(p).Life; got != tc.want {
				t.Errorf("life = %d, want %d", got, tc.want)
			}
		})
	}
}

// Echo:X (Volcano Hellion, X = your life total): the echo cost is X generic
// mana, so paying it takes one mana per life point.
func TestEchoXCostsYourLifeTotal(t *testing.T) {
	t.Parallel()

	g, p, other := newTwoPlayerGameOn(t, scenarioDB(t))
	libraryCards(t, g, p, 8)
	libraryCards(t, g, other, 8)
	g.Player(p).Life = 3
	hellion := g.NewCard(corpusCard(t, "Volcano Hellion"), p, engine.Battlefield)
	g.SetTurnState(2, p, engine.Untap)
	c := engine.NewScriptedController()
	c.QueueConfirmPayCost(true)
	for range 3 {
		c.QueuePayGeneric(mana.ShardR)
	}
	toUpkeep(t, g, c, p, mana.Red, 3)
	if got := g.Card(hellion).Zone; got != engine.Battlefield {
		t.Errorf("after paying {3} the Hellion is in %v, want Battlefield", got)
	}
	if left := g.Player(p).ManaPool.Total(); left != 0 {
		t.Errorf("mana left = %d, want 0: the echo cost was X = 3", left)
	}
}
