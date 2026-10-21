package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
	"github.com/jczastkiewicz/crucible/internal/mana"
)

// resolvePutCounterWith casts an ETB creature running DB$ PutCounter with
// params, its counters preset in hand, on a controller the caller queued.
func resolvePutCounterWith(t *testing.T, params string, c *engine.ScriptedController, preset func(*engine.Game, *engine.Card, engine.PlayerID)) (*engine.Game, engine.PlayerID, engine.CardID, error) {
	t.Helper()
	g, p, _ := newTwoPlayerGame(t)
	g.Player(p).ManaPool.Add(mana.Green, 1)
	host := g.NewCard(etbChainDef(t, "Test Put Kinds", "DB$ PutCounter | "+params), p, engine.Hand)
	preset(g, g.Card(host), p)
	if !g.CastSpell(p, host, c) {
		t.Fatal("CastSpell failed")
	}
	return g, p, host, g.ResolveStack(engine.NewRegistry(), c)
}

func twoKinds(_ *engine.Game, h *engine.Card, _ engine.PlayerID) {
	h.Counters.Add(engine.P1P1, 1)
	h.Counters.Add(engine.Stun, 2)
}

func TestPutCounterExistingCounterKinds(t *testing.T) {
	t.Parallel()

	t.Run("one kind is put without asking", func(t *testing.T) {
		t.Parallel()
		g, _, host, err := resolvePutCounterWith(t, "Defined$ Self | CounterType$ ExistingCounter | CounterNum$ 1", engine.NewScriptedController(),
			func(_ *engine.Game, h *engine.Card, _ engine.PlayerID) { h.Counters.Add(engine.P1P1, 1) })
		if err != nil {
			t.Fatal(err)
		}
		if n := g.Card(host).Counters.Count(engine.P1P1); n != 2 {
			t.Errorf("P1P1 = %d, want 2", n)
		}
	})
	t.Run("several kinds are chosen between", func(t *testing.T) {
		t.Parallel()
		c := engine.NewScriptedController()
		c.QueueOption(1)
		g, _, host, err := resolvePutCounterWith(t, "Defined$ Self | CounterType$ ExistingCounter | CounterNum$ 1", c, twoKinds)
		if err != nil {
			t.Fatal(err)
		}
		if n := g.Card(host).Counters.Count(engine.Stun); n != 3 {
			t.Errorf("Stun = %d, want 3 (the second kind was chosen)", n)
		}
	})
	t.Run("each existing kind", func(t *testing.T) {
		t.Parallel()
		g, _, host, err := resolvePutCounterWith(t, "Defined$ Self | CounterType$ ExistingCounter | EachExistingCounter$ True | CounterNum$ 1", engine.NewScriptedController(), twoKinds)
		if err != nil {
			t.Fatal(err)
		}
		if a, b := g.Card(host).Counters.Count(engine.P1P1), g.Card(host).Counters.Count(engine.Stun); a != 2 || b != 3 {
			t.Errorf("P1P1, Stun = %d, %d; want 2, 3", a, b)
		}
	})
	t.Run("no kind puts nothing", func(t *testing.T) {
		t.Parallel()
		g, _, host, err := resolvePutCounterWith(t, "Defined$ Self | CounterType$ ExistingCounter | CounterNum$ 1", engine.NewScriptedController(),
			func(*engine.Game, *engine.Card, engine.PlayerID) {})
		if err != nil {
			t.Fatal(err)
		}
		if n := g.Card(host).Counters.Total(); n != 0 {
			t.Errorf("counters = %d, want 0", n)
		}
	})
	t.Run("on each other creature", func(t *testing.T) {
		t.Parallel()
		c := engine.NewScriptedController()
		c.QueueOption(0)
		var other engine.CardID
		g, _, _, err := resolvePutCounterWith(t, "Defined$ Self | CounterType$ ExistingCounter | CounterNum$ 1 | PutOnEachOther$ Creature.Other", c,
			func(g *engine.Game, h *engine.Card, p engine.PlayerID) {
				twoKinds(g, h, p)
				other = g.NewCard(creatureDefPT(t, "1", "1"), p, engine.Battlefield)
			})
		if err != nil {
			t.Fatal(err)
		}
		if n := g.Card(other).Counters.Count(engine.P1P1); n != 1 {
			t.Errorf("other creature P1P1 = %d, want 1", n)
		}
	})
	t.Run("on a defined card", func(t *testing.T) {
		t.Parallel()
		g, _, host, err := resolvePutCounterWith(t, "Defined$ Self | CounterType$ ExistingCounter | CounterNum$ 1 | PutOnDefined$ Self", engine.NewScriptedController(),
			func(_ *engine.Game, h *engine.Card, _ engine.PlayerID) { h.Counters.Add(engine.P1P1, 1) })
		if err != nil {
			t.Fatal(err)
		}
		if n := g.Card(host).Counters.Count(engine.P1P1); n != 2 {
			t.Errorf("P1P1 = %d, want 2", n)
		}
	})
}

func TestPutCounterCountersListAndUpTo(t *testing.T) {
	t.Parallel()

	t.Run("split amount between listed kinds", func(t *testing.T) {
		t.Parallel()
		c := engine.NewScriptedController()
		c.QueueNumberChoice(1)
		g, _, host, err := resolvePutCounterWith(t, "Defined$ Self | CounterTypes$ P1P1,Stun | CounterNum$ 3 | SplitAmount$ True", c,
			func(*engine.Game, *engine.Card, engine.PlayerID) {})
		if err != nil {
			t.Fatal(err)
		}
		if a, b := g.Card(host).Counters.Count(engine.P1P1), g.Card(host).Counters.Count(engine.Stun); a != 1 || b != 2 {
			t.Errorf("P1P1, Stun = %d, %d; want 1, 2", a, b)
		}
	})
	t.Run("split amount outside the remainder", func(t *testing.T) {
		t.Parallel()
		c := engine.NewScriptedController()
		c.QueueNumberChoice(9)
		if _, _, _, err := resolvePutCounterWith(t, "Defined$ Self | CounterTypes$ P1P1,Stun | CounterNum$ 3 | SplitAmount$ True", c,
			func(*engine.Game, *engine.Card, engine.PlayerID) {}); err == nil {
			t.Error("a split larger than the amount was accepted")
		}
	})
	t.Run("each type is not resolvable", func(t *testing.T) {
		t.Parallel()
		if _, _, _, err := resolvePutCounterWith(t, "Defined$ Self | CounterTypes$ EachType | CounterNum$ 1", engine.NewScriptedController(),
			func(*engine.Game, *engine.Card, engine.PlayerID) {}); err == nil {
			t.Error("CounterTypes$ EachType resolved")
		}
	})
	t.Run("up to chooses a number", func(t *testing.T) {
		t.Parallel()
		c := engine.NewScriptedController()
		c.QueueNumberChoice(2)
		g, _, host, err := resolvePutCounterWith(t, "Defined$ Self | CounterType$ P1P1 | CounterNum$ 3 | UpTo$ True | UpToMin$ 1", c,
			func(*engine.Game, *engine.Card, engine.PlayerID) {})
		if err != nil {
			t.Fatal(err)
		}
		if n := g.Card(host).Counters.Count(engine.P1P1); n != 2 {
			t.Errorf("P1P1 = %d, want 2", n)
		}
	})
	t.Run("up to outside its range", func(t *testing.T) {
		t.Parallel()
		c := engine.NewScriptedController()
		c.QueueNumberChoice(9)
		if _, _, _, err := resolvePutCounterWith(t, "Defined$ Self | CounterType$ P1P1 | CounterNum$ 3 | UpTo$ True", c,
			func(*engine.Game, *engine.Card, engine.PlayerID) {}); err == nil {
			t.Error("an out-of-range number was accepted")
		}
	})
	t.Run("per-defined amount unresolvable", func(t *testing.T) {
		t.Parallel()
		if _, _, _, err := resolvePutCounterWith(t, "Defined$ Self | CounterType$ P1P1 | CounterNumPerDefined$ Bogus", engine.NewScriptedController(),
			func(*engine.Game, *engine.Card, engine.PlayerID) {}); err == nil {
			t.Error("an unresolvable CounterNumPerDefined$ resolved")
		}
	})
}
