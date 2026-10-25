package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
)

// Topography Tracker and Leader, Super-Genius: a creature that would explore
// or connive instead runs the replacement's ability with Defined$ ReplacedCard
// naming it, and that nested Explore or Connive is not replaced again by the
// same line (the hasRun guard), so the chain ends instead of recursing.
func TestExploreReplacementRunsOnceAroundTheNestedExplore(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGame(t)
	g.NewCard(planarDef(t, "Tracker", "Creature Elf", nil, []string{
		"Event$ Explore | ActiveZones$ Battlefield | ValidExplorer$ Creature.YouCtrl | ReplaceWith$ Again",
	}, "Again", "DB$ Explore | Defined$ ReplacedCard | Num$ 2"), p, engine.Battlefield)

	actor := resolveLine(t, g, p, engine.NewScriptedController(), "DB$ Explore | Defined$ Self")
	// The library is empty, so every explore puts a +1/+1 counter: the nested
	// two, and none for the replaced one.
	if got := g.Card(actor).Counters.Count(engine.P1P1); got != 2 {
		t.Errorf("counters = %d, want 2", got)
	}
}

func TestConniveReplacementDrawsFirstThenConnivesOnce(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGame(t)
	g.NewCard(planarDef(t, "Leader", "Creature Elf", nil, []string{
		"Event$ Connive | ActiveZones$ Battlefield | ValidCard$ Creature.YouCtrl | ReplaceWith$ Gain",
	}, "Gain", "DB$ GainLife | Defined$ You | LifeAmount$ 1 | SubAbility$ Again",
		"Again", "DB$ Connive | Defined$ ReplacedCard"), p, engine.Battlefield)

	before := g.Player(p).Life
	resolveLine(t, g, p, engine.NewScriptedController(), "DB$ Connive | Defined$ Self")
	if got := g.Player(p).Life - before; got != 1 {
		t.Errorf("life gained = %d, want 1 (the replacement ran once)", got)
	}
}
