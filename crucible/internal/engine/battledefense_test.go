package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
	"github.com/jczastkiewicz/crucible/internal/mana"
)

// drainingController empties a Battle's defense the moment it is asked who
// protects it -- inside the state-based-action pass that follows the Battle
// entering, while its ETB trigger is on the stack.
type drainingController struct {
	*engine.ScriptedController
}

func (d drainingController) ChooseBattleProtector(g *engine.Game, p engine.PlayerID, battle engine.CardID, eligible []engine.PlayerID) engine.PlayerID {
	g.Card(battle).Counters.Add(engine.Defense, -g.Card(battle).Counters.Count(engine.Defense))
	return d.ScriptedController.ChooseBattleProtector(g, p, battle, eligible)
}

// CR 704.5v: a Battle at zero defense that is the source of an ability that
// has triggered and not yet left the stack stays on the battlefield until
// that ability resolves (GameAction.java:1705-1710, hasSourceOnStack with a
// trigger predicate).
func TestZeroDefenseBattleWithItsTriggerOnTheStackSurvivesUntilItResolves(t *testing.T) {
	t.Parallel()

	g, p, other := newTwoPlayerGameOn(t, scenarioDB(t))
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).ManaPool.Add(mana.White, 1)
	g.Player(p).ManaPool.AddColorless(2)
	battle := g.NewCard(corpusCard(t, "Invasion of Belenon"), p, engine.Hand)
	sc := engine.NewScriptedController()
	sc.QueuePayGeneric(mana.ShardC)
	sc.QueuePayGeneric(mana.ShardC)
	sc.QueueBattleProtector(other)
	c := drainingController{sc}
	var sink recordingSink
	g.SetSink(&sink)

	if !g.CastSpell(p, battle, c) {
		t.Fatal("CastSpell failed casting Invasion of Belenon")
	}
	if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	// The token enters while the Battle is still on the battlefield, so the
	// Battle's own move to the graveyard comes after the token's entry.
	tokenEntered, battleLeft := -1, -1
	for i, e := range sink.events {
		if e.Kind != engine.ZoneChanged {
			continue
		}
		moved := e.Source == battle || e.Target == engine.CardEntity(battle)
		switch {
		case e.To == engine.Battlefield && !moved:
			tokenEntered = i
		case e.To == engine.Graveyard && moved:
			battleLeft = i
		}
	}
	if tokenEntered < 0 || battleLeft < 0 || battleLeft < tokenEntered {
		t.Errorf("token entered at event %d, Battle left at %d: want the Battle to leave after its trigger resolved", tokenEntered, battleLeft)
	}
	if got := g.Card(battle).Zone; got != engine.Graveyard {
		t.Errorf("Battle zone = %v after its trigger resolved, want Graveyard", got)
	}
	if got := len(g.Zone(engine.Battlefield, p).Cards()); got != 1 {
		t.Errorf("battlefield has %d cards, want just the Knight token the trigger made", got)
	}
}
