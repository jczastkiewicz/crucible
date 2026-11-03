package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
	"github.com/jczastkiewicz/crucible/internal/mana"
)

// Encore (CR 702.141a, CardFactoryUtil.java:3478-3508): Impulsive Pilferer's
// Encore {3}{R} exiles it from the graveyard for a hasty token copy per
// opponent that must attack that opponent this turn and is sacrificed at the
// beginning of the next end step.
func TestEncoreTokenAttacksItsOpponentAndIsSacrificed(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGameOn(t, scenarioDB(t))
	g.SetTurnState(1, p, engine.Main1)
	pilferer := g.NewCard(corpusCard(t, "Impulsive Pilferer"), p, engine.Graveyard)
	g.Player(p).ManaPool.Add(mana.Red, 4)
	c := engine.NewScriptedController()
	for range 3 {
		c.QueuePayGeneric(mana.ShardR)
	}
	if !g.ActivateAbility(p, pilferer, 0, c) {
		t.Fatal("Encore {3}{R} could not be activated from the graveyard")
	}
	if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
		t.Fatal(err)
	}
	engine.CheckStateBasedActions(g, c)
	if got := g.Card(pilferer).Zone; got != engine.Exile {
		t.Errorf("Impulsive Pilferer in %v, want exile", got)
	}
	var token engine.CardID
	for _, id := range g.Zone(engine.Battlefield, p).Cards() {
		if g.Card(id).IsToken && g.Card(id).Name() == "Impulsive Pilferer" {
			token = id
		}
	}
	if token == engine.NoCard {
		t.Fatal("no token copy entered")
	}
	if !g.Card(token).HasKeyword("Haste") {
		t.Error("the encore token has no haste")
	}
	g.SetTurnState(1, p, engine.DeclareAttackers)
	ac := engine.NewScriptedController()
	ac.QueueAttackers(nil)
	_, err := g.DeclareCombatAttackers(ac)
	wantIllegal(t, err, "CR 508.1d")
	g.SetTurnState(1, p, engine.Main2)
	toStep(t, g, c, p, engine.EndOfTurn, nil)
	if got := g.Card(token).Zone; got == engine.Battlefield {
		t.Error("the encore token is still on the battlefield after the end step")
	}
}
