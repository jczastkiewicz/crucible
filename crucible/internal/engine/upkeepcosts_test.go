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
