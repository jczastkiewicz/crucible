package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/carddb"
	"github.com/jczastkiewicz/crucible/internal/carddb/compile"
	"github.com/jczastkiewicz/crucible/internal/cardtype"
	"github.com/jczastkiewicz/crucible/internal/engine"
	"github.com/jczastkiewicz/crucible/internal/mana"
)

// lifeSagaDef is a three-chapter Saga whose chapters gain 1, 2 and 3 life.
func lifeSagaDef(t *testing.T) *compile.Card {
	t.Helper()

	raw := &carddb.Card{Filename: "life-saga"}
	raw.Faces[0].Present = true
	raw.Faces[0].Name = "Life Saga"
	raw.Faces[0].Type = cardtype.Parse(attachmentTypeRegistry(t), "Enchantment Saga")
	raw.Faces[0].ManaCost = mana.MustParse("1 W")
	raw.Faces[0].Keywords = []string{"Chapter:3:One,Two,Three"}
	raw.Faces[0].SVars.Set("One", "DB$ GainLife | Defined$ You | LifeAmount$ 1")
	raw.Faces[0].SVars.Set("Two", "DB$ GainLife | Defined$ You | LifeAmount$ 2")
	raw.Faces[0].SVars.Set("Three", "DB$ GainLife | Defined$ You | LifeAmount$ 3")
	c, err := compile.Compile(raw)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	return c
}

// CR 714: a Saga enters with a lore counter, gets one at each of its
// controller's precombat main phases, runs the chapter its counters reach, and
// is sacrificed once the last chapter has resolved.
func TestSagaRunsItsChaptersAndIsSacrificed(t *testing.T) {
	t.Parallel()

	g, p, other := newTwoPlayerGameOn(t, scenarioDB(t))
	libraryCards(t, g, p, 8)
	libraryCards(t, g, other, 8)
	g.SetTurnState(2, p, engine.Main1)
	g.Player(p).Life = 10
	saga := g.NewCard(lifeSagaDef(t), p, engine.Hand)
	g.Player(p).ManaPool.Add(mana.White, 2)
	c := engine.NewScriptedController()
	c.QueuePayGeneric(mana.ShardW)
	castThenResolve(t, g, p, saga, c)

	if got := g.Card(saga).Counters.Count(engine.Lore); got != 1 {
		t.Fatalf("lore counters on entering = %d, want 1", got)
	}
	if got := g.Player(p).Life; got != 11 {
		t.Errorf("life after chapter I = %d, want 11", got)
	}

	toStep(t, g, c, p, engine.Main1, nil)
	if got := g.Card(saga).Counters.Count(engine.Lore); got != 2 {
		t.Fatalf("lore counters after the next precombat main = %d, want 2", got)
	}
	if got := g.Player(p).Life; got != 13 {
		t.Errorf("life after chapter II = %d, want 13", got)
	}

	toStep(t, g, c, p, engine.Main1, nil)
	if got := g.Player(p).Life; got != 16 {
		t.Errorf("life after chapter III = %d, want 16", got)
	}
	if got := g.Card(saga).Zone; got != engine.Graveyard {
		t.Errorf("after chapter III resolved the Saga is in %v, want Graveyard", got)
	}
}

// A Saga is not sacrificed while its last chapter ability is still on the stack.
func TestSagaStaysUntilItsFinalChapterLeavesTheStack(t *testing.T) {
	t.Parallel()

	g, p, other := newTwoPlayerGameOn(t, scenarioDB(t))
	libraryCards(t, g, p, 8)
	libraryCards(t, g, other, 8)
	g.SetTurnState(2, p, engine.Main1)
	saga := g.NewCard(lifeSagaDef(t), p, engine.Hand)
	g.Player(p).ManaPool.Add(mana.White, 2)
	c := engine.NewScriptedController()
	c.QueuePayGeneric(mana.ShardW)
	castThenResolve(t, g, p, saga, c)
	toStep(t, g, c, p, engine.Main1, nil)

	// The third precombat main: the lore counter has been added and chapter
	// III's trigger is on the stack, unresolved.
	for {
		g.AdvancePhase(c)
		if g.ActivePhase() == engine.Main1 && g.ActivePlayer() == p {
			break
		}
		if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
			t.Fatal(err)
		}
	}
	if got := g.Card(saga).Counters.Count(engine.Lore); got != 3 {
		t.Fatalf("lore counters = %d, want 3", got)
	}
	engine.CheckStateBasedActions(g, c)
	if got := g.Card(saga).Zone; got != engine.Battlefield {
		t.Fatalf("Saga is in %v with chapter III on the stack, want Battlefield", got)
	}
	if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
		t.Fatal(err)
	}
	if got := g.Card(saga).Zone; got != engine.Graveyard {
		t.Errorf("Saga is in %v after chapter III resolved, want Graveyard", got)
	}
}

// counterAddedWatcherDef is a creature that can put two +1/+1 counters on itself and
// watches for them: Mode$ CounterAdded gains 1 life per counter, Mode$
// CounterAddedOnce gains 10 for the whole placement.
func counterAddedWatcherDef(t *testing.T) *compile.Card {
	t.Helper()

	raw := &carddb.Card{Filename: "counter-watcher"}
	raw.Faces[0].Present = true
	raw.Faces[0].Name = "Counter Watcher"
	raw.Faces[0].Type = cardtype.Parse(attachmentTypeRegistry(t), "Creature Elf")
	raw.Faces[0].Power, raw.Faces[0].Toughness = "1", "1"
	raw.Faces[0].Abilities = []string{"AB$ PutCounter | Cost$ 0 | Defined$ Self | CounterType$ P1P1 | CounterNum$ 2"}
	raw.Faces[0].Triggers = []string{
		"Mode$ CounterAdded | ValidCard$ Card.Self | CounterType$ P1P1 | TriggerZones$ Battlefield | Execute$ Each",
		"Mode$ CounterAddedOnce | ValidCard$ Card.Self | CounterType$ P1P1 | TriggerZones$ Battlefield | Execute$ Once",
	}
	raw.Faces[0].SVars.Set("Each", "DB$ GainLife | Defined$ You | LifeAmount$ 1")
	raw.Faces[0].SVars.Set("Once", "DB$ GainLife | Defined$ You | LifeAmount$ 10")
	c, err := compile.Compile(raw)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	return c
}

// Card.addCounter (Card.java:1796-1822) fires CounterAdded once per counter and
// CounterAddedOnce once per placement.
func TestCounterAddedTriggersPerCounterAndOncePerPlacement(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGameOn(t, scenarioDB(t))
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life = 10
	watcher := g.NewCard(counterAddedWatcherDef(t), p, engine.Battlefield)
	g.Card(watcher).SummonSick = false
	c := engine.NewScriptedController()
	if !g.ActivateAbility(p, watcher, 0, c) {
		t.Fatal("could not activate the ability")
	}
	if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if got := g.Card(watcher).Counters.Count(engine.P1P1); got != 2 {
		t.Fatalf("counters = %d, want 2", got)
	}
	if got, want := g.Player(p).Life, 10+2*1+10; got != want {
		t.Errorf("life = %d, want %d: two CounterAdded triggers and one CounterAddedOnce", got, want)
	}
}
