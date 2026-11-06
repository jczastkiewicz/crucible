package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
)

// TestProliferateEffectAddsOneOfEachKind proves CR 701.34a: every chosen
// permanent or player gets one more of each counter kind it already has,
// and an unchosen one is untouched.
func TestProliferateEffectAddsOneOfEachKind(t *testing.T) {
	t.Parallel()

	g, p, other := newTwoPlayerGame(t)
	mine := g.NewCard(creatureDefPT(t, "2", "2"), p, engine.Battlefield)
	g.Card(mine).Counters.Add(engine.P1P1, 1)
	g.Card(mine).Counters.Add(engine.Charge, 2)
	theirs := g.NewCard(creatureDefPT(t, "2", "2"), other, engine.Battlefield)
	g.Card(theirs).Counters.Add(engine.P1P1, 1)
	g.Player(other).Counters.Add(engine.Poison, 1)

	c := engine.NewScriptedController()
	c.QueueEntityChoice([]engine.EntityID{engine.PlayerEntity(other), engine.CardEntity(mine)})
	if _, err := castETBChain(t, g, p, etbChainDef(t, "Test Proliferate", "DB$ Proliferate"), c); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if got := g.Card(mine).Counters.Count(engine.P1P1); got != 2 {
		t.Errorf("mine P1P1 = %d, want 2", got)
	}
	if got := g.Card(mine).Counters.Count(engine.Charge); got != 3 {
		t.Errorf("mine CHARGE = %d, want 3", got)
	}
	if got := g.Player(other).Counters.Count(engine.Poison); got != 2 {
		t.Errorf("opponent POISON = %d, want 2", got)
	}
	if got := g.Card(theirs).Counters.Count(engine.P1P1); got != 1 {
		t.Errorf("unchosen P1P1 = %d, want 1", got)
	}
}

// TestProliferateEffectAmountRepeats proves Amount$ runs the whole choice
// that many times.
func TestProliferateEffectAmountRepeats(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGame(t)
	mine := g.NewCard(creatureDefPT(t, "2", "2"), p, engine.Battlefield)
	g.Card(mine).Counters.Add(engine.P1P1, 1)

	c := engine.NewScriptedController()
	c.QueueEntityChoice([]engine.EntityID{engine.CardEntity(mine)})
	c.QueueEntityChoice([]engine.EntityID{engine.CardEntity(mine)})
	if _, err := castETBChain(t, g, p, etbChainDef(t, "Test Proliferate Twice", "DB$ Proliferate | Amount$ 2"), c); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if got := g.Card(mine).Counters.Count(engine.P1P1); got != 3 {
		t.Errorf("P1P1 = %d, want 3", got)
	}
}

// TestProliferateEffectRejectsEntityWithoutCounters proves only entities
// that already have a counter are offered (GO-7).
func TestProliferateEffectRejectsEntityWithoutCounters(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGame(t)
	bare := g.NewCard(creatureDefPT(t, "2", "2"), p, engine.Battlefield)

	c := engine.NewScriptedController()
	c.QueueEntityChoice([]engine.EntityID{engine.CardEntity(bare)})
	if _, err := castETBChain(t, g, p, etbChainDef(t, "Test Proliferate Bad", "DB$ Proliferate"), c); err == nil {
		t.Fatal("ResolveStack succeeded, want an error for a permanent with no counters")
	}
}

// TestProliferateRunsCounterReplacementsAndTriggers proves each proliferated
// counter goes through the AddCounter replacements and fires CounterAdded:
// Hardened Scales makes a creature's one more +1/+1 counter two, Winding
// Constrictor makes the player's one more energy two, and a CounterAddedOnce
// watcher fires for the creature.
func TestProliferateRunsCounterReplacementsAndTriggers(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGameOn(t, scenarioDB(t))
	g.SetTurnState(1, p, engine.Main1)
	g.NewCard(corpusCard(t, "Hardened Scales"), p, engine.Battlefield)
	g.NewCard(planarDef(t, "Test Watcher", "Enchantment",
		[]string{"Mode$ CounterAddedOnce | ValidCard$ Creature.YouCtrl | CounterType$ P1P1 | TriggerZones$ Battlefield | Execute$ TrigGain"},
		nil, "TrigGain", "DB$ GainLife | Defined$ You | LifeAmount$ X", "X", "TriggerCount$Amount"), p, engine.Battlefield)
	bear := g.NewCard(creatureDefPT(t, "2", "2"), p, engine.Battlefield)
	g.Card(bear).Counters.Add(engine.P1P1, 1)
	g.Player(p).Counters.Add(engine.Energy, 1)

	ctl := engine.NewScriptedController()
	ctl.QueueEntityChoice([]engine.EntityID{engine.PlayerEntity(p), engine.CardEntity(bear)})
	life := g.Player(p).Life
	if _, err := resolveNow(t, g, p, ctl, nil, "DB$ Proliferate"); err != nil {
		t.Fatalf("Proliferate: %v", err)
	}
	if got := g.Card(bear).Counters.Count(engine.P1P1); got != 3 {
		t.Errorf("P1P1 = %d, want 3 (1 + 1 + Hardened Scales' 1)", got)
	}
	if got := g.Player(p).Life - life; got != 2 {
		t.Errorf("CounterAddedOnce gained %d, want 2", got)
	}

	g.NewCard(corpusCard(t, "Winding Constrictor"), p, engine.Battlefield)
	ctl.QueueEntityChoice([]engine.EntityID{engine.PlayerEntity(p)})
	if _, err := resolveNow(t, g, p, ctl, nil, "DB$ Proliferate"); err != nil {
		t.Fatalf("Proliferate with Winding Constrictor: %v", err)
	}
	if got := g.Player(p).Counters.Count(engine.Energy); got != 4 {
		t.Errorf("energy = %d, want 4 (2 + 1 + Winding Constrictor's 1)", got)
	}
}

// TestTekuthalProliferatesTwice proves Tekuthal, Inquiry Dominus's Event$
// Proliferate ReplaceEffect doubles the passes: the choice is asked twice.
func TestTekuthalProliferatesTwice(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGameOn(t, scenarioDB(t))
	g.SetTurnState(1, p, engine.Main1)
	g.NewCard(corpusCard(t, "Tekuthal, Inquiry Dominus"), p, engine.Battlefield)
	bear := g.NewCard(creatureDefPT(t, "2", "2"), p, engine.Battlefield)
	g.Card(bear).Counters.Add(engine.P1P1, 1)

	ctl := engine.NewScriptedController()
	ctl.QueueEntityChoice([]engine.EntityID{engine.CardEntity(bear)})
	ctl.QueueEntityChoice([]engine.EntityID{engine.CardEntity(bear)})
	if _, err := resolveNow(t, g, p, ctl, nil, "DB$ Proliferate"); err != nil {
		t.Fatalf("Proliferate: %v", err)
	}
	if got := g.Card(bear).Counters.Count(engine.P1P1); got != 3 {
		t.Errorf("P1P1 = %d, want 3", got)
	}
}

// TestProliferateReplacedOutright proves an Event$ Proliferate line whose
// ReplaceWith$ is an ordinary ability replaces the whole proliferate: the
// ability runs and no counter moves.
func TestProliferateReplacedOutright(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGame(t)
	g.SetTurnState(1, p, engine.Main1)
	g.NewCard(planarDef(t, "Replacer", "Creature Elf", nil, []string{
		"Event$ Proliferate | ActiveZones$ Battlefield | ValidPlayer$ You | ReplaceWith$ Plus",
	}, "Plus", "DB$ GainLife | Defined$ You | LifeAmount$ 1"), p, engine.Battlefield)
	bear := g.NewCard(creatureDefPT(t, "2", "2"), p, engine.Battlefield)
	g.Card(bear).Counters.Add(engine.P1P1, 1)

	life := g.Player(p).Life
	if _, err := resolveNow(t, g, p, engine.NewScriptedController(), nil, "DB$ Proliferate"); err != nil {
		t.Fatalf("Proliferate: %v", err)
	}
	if got := g.Player(p).Life - life; got != 1 {
		t.Errorf("gained %d, want 1", got)
	}
	if got := g.Card(bear).Counters.Count(engine.P1P1); got != 1 {
		t.Errorf("P1P1 = %d, want 1 (untouched)", got)
	}
}
