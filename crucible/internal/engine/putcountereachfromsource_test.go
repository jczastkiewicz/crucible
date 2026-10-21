package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
)

func putEachFromSource(t *testing.T, cost, extra string) (*engine.Game, engine.CardID, engine.CardID) {
	t.Helper()

	g, p, _ := newTwoPlayerGameOn(t, scenarioDB(t))
	g.SetTurnState(1, p, engine.Main1)
	src := g.NewCard(creatureDefWithAbility(t, "Test Counter Giver",
		"AB$ PutCounter | Cost$ "+cost+" | ValidTgts$ Creature.Other | CounterType$ EachFromSource | EachFromSource$ Self"+extra), p, engine.Battlefield)
	g.Card(src).SummonSick = false
	g.Card(src).Counters.Add("P1P1", 2)
	g.Card(src).Counters.Add("LOYALTY", 3)
	target := g.NewCard(creatureDefPT(t, "1", "1"), p, engine.Battlefield)
	c := engine.NewScriptedController()
	c.QueueTargets([]engine.EntityID{engine.CardEntity(target)})
	if !g.ActivateAbility(p, src, 0, c) {
		t.Fatal("could not activate")
	}
	if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
		t.Fatal(err)
	}
	return g, src, target
}

// EachFromSource$ copies every kind the source has, at its count.
func TestPutCounterEachFromSourceCopiesEveryKind(t *testing.T) {
	t.Parallel()

	g, src, target := putEachFromSource(t, "T", "")
	if a, b := g.Card(target).Counters.Count("P1P1"), g.Card(target).Counters.Count("LOYALTY"); a != 2 || b != 3 {
		t.Errorf("target counters P1P1 %d LOYALTY %d, want 2 3", a, b)
	}
	if got := g.Card(src).Counters.Count("P1P1"); got != 2 {
		t.Errorf("the source lost counters: %d, want 2", got)
	}
}

// CounterNum$ replaces the count of each kind.
func TestPutCounterEachFromSourceCounterNumOverridesTheCount(t *testing.T) {
	t.Parallel()

	g, _, target := putEachFromSource(t, "T", " | CounterNum$ 1")
	if a, b := g.Card(target).Counters.Count("P1P1"), g.Card(target).Counters.Count("LOYALTY"); a != 1 || b != 1 {
		t.Errorf("target counters P1P1 %d LOYALTY %d, want 1 1", a, b)
	}
}

// Zack Fair: the source sacrificed to pay is read as it last was.
func TestPutCounterEachFromSourceReadsASacrificedSourceAsItLastWas(t *testing.T) {
	t.Parallel()

	g, src, target := putEachFromSource(t, "Sac<1/CARDNAME>", "")
	if g.Card(src).Zone == engine.Battlefield {
		t.Fatal("the source was not sacrificed")
	}
	if got := g.Card(target).Counters.Count("P1P1"); got != 2 {
		t.Errorf("target P1P1 counters = %d, want 2", got)
	}
}
