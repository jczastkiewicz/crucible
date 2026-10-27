package engine_test

import (
	"slices"
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
)

// Shape of the Wiitigo: each of its controller's upkeeps adds a +1/+1 counter
// when the enchanted creature attacked or blocked since the end of the last
// upkeep, else removes one (CountersPutOrRemoveEffect.java:116-124,
// CardProperty.java:1087-1091). A card starts with no flags set, so the first
// upkeep counts as "attacked".
func shapeGame(t *testing.T) (*engine.Game, engine.PlayerID, engine.CardID, *engine.ScriptedController) {
	t.Helper()
	g, p, _ := newTwoPlayerGame(t)
	creature := g.NewCard(creatureDefPT(t, "2", "2"), p, engine.Battlefield)
	g.Card(creature).Counters.Add(engine.CounterType("P1P1"), 6)
	shape := g.NewCard(corpusCard(t, "Shape of the Wiitigo"), p, engine.Battlefield)
	g.Attach(shape, creature)
	return g, p, creature, engine.NewScriptedController()
}

func shapeCounters(t *testing.T, attackTurn int) []int {
	t.Helper()
	g, p, creature, c := shapeGame(t)
	var seen []int
	for turn := 1; turn <= 7; turn++ {
		advanceToPhase(t, g, c, turn, engine.Main1)
		if g.ActivePlayer() != p {
			continue
		}
		seen = append(seen, g.Card(creature).Counters.Count(engine.CounterType("P1P1")))
		if turn == attackTurn {
			advanceToPhase(t, g, c, turn, engine.DeclareAttackers)
			declareAttacking(t, g, creature)
		}
	}
	return seen
}

func TestShapeOfTheWiitigoRemovesACounterWhenTheCreatureStayedHome(t *testing.T) {
	t.Parallel()

	got := shapeCounters(t, 0)
	// Turn 1 has no upkeep trigger; turn 3's upkeep sees no flags yet (the
	// first upkeep counts as "attacked"); later upkeeps remove one each.
	want := []int{6, 7, 6, 5}
	if !slices.Equal(got, want) {
		t.Errorf("counters at each of the controller's turns = %v, want %v", got, want)
	}
}

func TestShapeOfTheWiitigoAddsACounterAfterTheCreatureAttacked(t *testing.T) {
	t.Parallel()

	got := shapeCounters(t, 3)
	want := []int{6, 7, 8, 7}
	if !slices.Equal(got, want) {
		t.Errorf("counters at each of the controller's turns = %v, want %v", got, want)
	}
}
