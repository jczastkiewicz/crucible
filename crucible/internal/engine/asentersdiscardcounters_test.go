package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
)

// Indominus Rex, Alpha: "As it enters, discard any number of creature cards. It
// enters with a flying counter on it if a card discarded this way has flying."
// DBPutCounters has no ETB$, but the counters it puts during the Moved
// replacement survive the move (GameAction.java:179 clears before :330).

func TestAsEntersDiscardThenKeywordCountersOfTheDiscarded(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGameOn(t, scenarioDB(t))
	rex := g.NewCard(scriptDef(t, "Test Rex", "Creature Dinosaur",
		"R:Event$ Moved | ValidCard$ Card.Self | Destination$ Battlefield | ReplaceWith$ Discard | ReplacementResult$ Updated",
		"SVar:Discard:DB$ Discard | DiscardValid$ Creature | AnyNumber$ True | Optional$ True | Mode$ TgtChoose | RememberDiscarded$ True | SubAbility$ DBPutCounters",
		"SVar:DBPutCounters:DB$ PutCounter | SharedKeywords$ Flying & Vigilance & Reach | SharedKeywordsDefined$ Remembered | CounterNum$ 1 | SubAbility$ DBCleanup",
		"SVar:DBCleanup:DB$ Cleanup | ClearRemembered$ True"), p, engine.Hand)
	angel := g.NewCard(scriptDef(t, "Test Angel", "Creature Angel", "K:Flying", "K:Vigilance"), p, engine.Hand)
	g.NewCard(scriptDef(t, "Test Bear", "Creature Bear"), p, engine.Hand)

	c := engine.NewScriptedController()
	c.QueueCardChoice([]engine.CardID{angel})
	if err := castEntrant(t, g, p, rex, c); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if z := g.Card(rex).Zone; z != engine.Battlefield {
		t.Fatalf("Rex zone = %v, want Battlefield", z)
	}
	if z := g.Card(angel).Zone; z != engine.Graveyard {
		t.Errorf("angel zone = %v, want Graveyard", z)
	}
	counters := &g.Card(rex).Counters
	for ct, want := range map[engine.CounterType]int{"FLYING": 1, "VIGILANCE": 1, "REACH": 0} {
		if got := counters.Count(ct); got != want {
			t.Errorf("%s counters = %d, want %d", ct, got, want)
		}
	}
}
