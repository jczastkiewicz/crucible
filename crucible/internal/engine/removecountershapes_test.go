package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
	"github.com/jczastkiewicz/crucible/internal/mana"
)

// resolveRemoveCounterWith casts an ETB creature running DB$ RemoveCounter
// with params, its Counters preset, on a controller the caller queued.
func resolveRemoveCounterWith(t *testing.T, params string, c *engine.ScriptedController, preset func(*engine.Game, *engine.Card, engine.PlayerID)) (*engine.Game, engine.PlayerID, engine.CardID, error) {
	t.Helper()
	g := newGame(t, "a", "b")
	p := g.Players()[0]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life, g.Player(g.Players()[1]).Life = 20, 20
	g.Player(p).ManaPool.Add(mana.Green, 1)
	host := g.NewCard(etbRemoveCounterTriggerDefParams(t, "Test Remove Shape", params), p, engine.Hand)
	preset(g, g.Card(host), p)
	if !g.CastSpell(p, host, c) {
		t.Fatal("CastSpell failed")
	}
	return g, p, host, g.ResolveStack(engine.NewRegistry(), c)
}

func TestRemoveCounterOptionalDeclinedRemovesNothing(t *testing.T) {
	t.Parallel()
	c := engine.NewScriptedController()
	c.QueueConfirmEffect(false)
	g, _, host, err := resolveRemoveCounterWith(t, "CounterType$ P1P1 | CounterNum$ 1 | Defined$ Self | Optional$ True", c,
		func(_ *engine.Game, h *engine.Card, _ engine.PlayerID) { h.Counters.Add(engine.P1P1, 2) })
	if err != nil {
		t.Fatal(err)
	}
	if n := g.Card(host).Counters.Count(engine.P1P1); n != 2 {
		t.Errorf("P1P1 = %d, want 2", n)
	}
}

func TestRemoveCounterAnyKindAsksKindAndNumber(t *testing.T) {
	t.Parallel()
	c := engine.NewScriptedController()
	c.QueueOption(0)
	c.QueueNumberChoice(2)
	g, _, host, err := resolveRemoveCounterWith(t, "CounterType$ Any | CounterNum$ 2 | Defined$ Self | RememberRemoved$ True | RememberAmount$ True", c,
		func(_ *engine.Game, h *engine.Card, _ engine.PlayerID) { h.Counters.Add(engine.P1P1, 3) })
	if err != nil {
		t.Fatal(err)
	}
	if n := g.Card(host).Counters.Count(engine.P1P1); n != 1 {
		t.Errorf("P1P1 = %d, want 1", n)
	}
	if got := g.Card(host).Memory.RememberedNumbers(); len(got) != 1 || got[0] != 2 {
		t.Errorf("remembered amount = %v, want [2]", got)
	}
}

func TestRemoveCounterAnyKindOnPlayerAnyAmountUpTo(t *testing.T) {
	t.Parallel()
	c := engine.NewScriptedController()
	c.QueueOption(0)
	c.QueueNumberChoice(0)
	g, p, _, err := resolveRemoveCounterWith(t, "CounterType$ Any | CounterNum$ Any | Defined$ You", c,
		func(g *engine.Game, _ *engine.Card, p engine.PlayerID) { g.Player(p).Counters.Add(engine.Energy, 3) })
	if err != nil {
		t.Fatal(err)
	}
	if n := g.Player(p).Counters.Count(engine.Energy); n != 3 {
		t.Errorf("Energy = %d, want 3 (the player chose none)", n)
	}

	c = engine.NewScriptedController()
	c.QueueOption(0)
	c.QueueNumberChoice(2)
	g, p, _, err = resolveRemoveCounterWith(t, "CounterType$ Any | CounterNum$ 2 | Defined$ You", c,
		func(g *engine.Game, _ *engine.Card, p engine.PlayerID) { g.Player(p).Counters.Add(engine.Energy, 3) })
	if err != nil {
		t.Fatal(err)
	}
	if n := g.Player(p).Counters.Count(engine.Energy); n != 1 {
		t.Errorf("Energy = %d, want 1", n)
	}
}

func TestRemoveCounterUpToAsksHowManyAndRemembers(t *testing.T) {
	t.Parallel()
	c := engine.NewScriptedController()
	c.QueueNumberChoice(2)
	g, _, host, err := resolveRemoveCounterWith(t, "CounterType$ P1P1 | CounterNum$ 3 | UpTo$ True | Defined$ Self | RememberRemoved$ True | RememberAmount$ True", c,
		func(_ *engine.Game, h *engine.Card, _ engine.PlayerID) { h.Counters.Add(engine.P1P1, 4) })
	if err != nil {
		t.Fatal(err)
	}
	if n := g.Card(host).Counters.Count(engine.P1P1); n != 2 {
		t.Errorf("P1P1 = %d, want 2", n)
	}
	if got := g.Card(host).Memory.RememberedNumbers(); len(got) != 1 || got[0] != 2 {
		t.Errorf("remembered amount = %v, want [2]", got)
	}

	c = engine.NewScriptedController()
	c.QueueNumberChoice(9)
	if _, _, _, err = resolveRemoveCounterWith(t, "CounterType$ P1P1 | CounterNum$ 3 | UpTo$ True | Defined$ Self", c,
		func(_ *engine.Game, h *engine.Card, _ engine.PlayerID) { h.Counters.Add(engine.P1P1, 4) }); err == nil {
		t.Error("an out-of-range number was accepted")
	}
}

func TestRemoveCounterRefusesUnreadShapes(t *testing.T) {
	t.Parallel()
	for _, params := range []string{
		"CounterNum$ 1 | Defined$ Self",
		"CounterType$ P1P1,M1M1 | CounterNum$ 1 | Defined$ Self",
		"CounterType$ P1P1 | CounterNum$ Bogus | Defined$ Self",
	} {
		_, _, _, err := resolveRemoveCounterWith(t, params, engine.NewScriptedController(), func(*engine.Game, *engine.Card, engine.PlayerID) {})
		if err == nil {
			t.Errorf("%q resolved, want an error", params)
		}
	}
}

// CounterType$ Any with CounterNum$ All reads the player's count of no kind
// (CountersRemoveEffect.java:92-100, 150-152: counterType stays null for Any,
// getCounters(null) is 0), so nothing comes off. No corpus line writes the
// pair; the port reproduces it rather than invent a meaning (PORT-7).
func TestRemoveCounterAnyKindWithAllAmountOnAPlayerRemovesNothing(t *testing.T) {
	t.Parallel()
	g, p, _, err := resolveRemoveCounterWith(t, "CounterType$ Any | CounterNum$ All | Defined$ You", engine.NewScriptedController(),
		func(g *engine.Game, _ *engine.Card, p engine.PlayerID) { g.Player(p).Counters.Add(engine.Energy, 3) })
	if err != nil {
		t.Fatal(err)
	}
	if n := g.Player(p).Counters.Count(engine.Energy); n != 3 {
		t.Errorf("Energy = %d, want 3", n)
	}
}
