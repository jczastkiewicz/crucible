package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
	"github.com/jczastkiewicz/crucible/internal/mana"
)

// You're in Command (ADR-0051): the targeted creature becomes its controller's
// commander and every other commander of theirs stops being one.
func TestYoureInCommandReplacesTheCommander(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGameOn(t, scenarioDB(t))
	g.SetTurnState(1, p, engine.Main1)
	first := g.NewCard(creatureDefPT(t, "2", "2"), p, engine.Battlefield)
	second := g.NewCard(creatureDefPT(t, "3", "3"), p, engine.Battlefield)
	for i, target := range []engine.CardID{first, second} {
		spell := g.NewCard(corpusCard(t, "You're in Command"), p, engine.Hand)
		g.Player(p).ManaPool.Add(mana.White, 2)
		c := engine.NewScriptedController()
		c.QueueTargets([]engine.EntityID{engine.CardEntity(target)})
		c.QueuePayGeneric(mana.ShardW)
		castThenResolve(t, g, p, spell, c)
		if err := g.TakePendingError(); err != nil {
			t.Fatalf("cast %d: pending error: %v", i, err)
		}
		if got := g.Commanders(p); len(got) != 1 || got[0] != target {
			t.Errorf("cast %d: commanders = %v, want only %v", i, got, target)
		}
	}
}
