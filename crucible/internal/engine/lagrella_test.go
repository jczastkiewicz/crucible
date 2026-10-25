package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
	"github.com/jczastkiewicz/crucible/internal/mana"
)

// Lagrella, the Magpie: "exile any number of other target creatures controlled
// by different players until Lagrella leaves the battlefield. When an exiled
// card enters under your control this way, put two +1/+1 counters on it." The
// ReturnAbility$ runs for the card that comes back under its caster's control
// and not for the one that goes back to the opponent (ReturnValid$
// Card.IsTriggerRemembered+YouCtrl, SpellAbilityEffect.java:878-902).
func TestLagrellaCountersOnlyTheCreatureReturningUnderItsController(t *testing.T) {
	t.Parallel()

	g, p, other := newTwoPlayerGameOn(t, scenarioDB(t))
	c := engine.NewScriptedController()
	g.SetTurnState(1, p, engine.Main1)
	ours := g.NewCard(corpusCard(t, "Grizzly Bears"), p, engine.Battlefield)
	theirs := g.NewCard(corpusCard(t, "Grizzly Bears"), other, engine.Battlefield)
	lagrella := g.NewCard(corpusCard(t, "Lagrella, the Magpie"), p, engine.Hand)
	g.Player(p).ManaPool.Add(mana.Green, 1)
	g.Player(p).ManaPool.Add(mana.White, 1)
	g.Player(p).ManaPool.Add(mana.Blue, 1)
	c.QueuePayGeneric(mana.ShardG)
	c.QueueTargets([]engine.EntityID{engine.CardEntity(ours), engine.CardEntity(theirs)})
	if !g.CastSpell(p, lagrella, c) {
		t.Fatalf("CastSpell failed: %v", g.TakePendingError())
	}
	if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if g.Card(ours).Zone != engine.Exile || g.Card(theirs).Zone != engine.Exile {
		t.Fatalf("zones after the trigger: ours %v, theirs %v, want both Exile", g.Card(ours).Zone, g.Card(theirs).Zone)
	}
	g.Move(lagrella, engine.Graveyard, p)
	engine.CheckStateBasedActions(g, c)
	if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
		t.Fatalf("ResolveStack after Lagrella left: %v", err)
	}
	counters := func(id engine.CardID) int { return g.Card(id).Counters.Count(engine.P1P1) }
	if g.Card(ours).Zone != engine.Battlefield || g.Card(theirs).Zone != engine.Battlefield {
		t.Fatalf("zones after Lagrella left: ours %v, theirs %v, want both Battlefield", g.Card(ours).Zone, g.Card(theirs).Zone)
	}
	if got := counters(ours); got != 2 {
		t.Errorf("the creature back under its caster has %d counters, want 2", got)
	}
	if got := counters(theirs); got != 0 {
		t.Errorf("the creature back under the opponent has %d counters, want 0", got)
	}
}
