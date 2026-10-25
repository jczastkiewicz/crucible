package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
)

// The Fallen: at your upkeep it deals 1 damage to each opponent it has dealt
// damage to this game (PlayerProperty wasDealtDamageThisGameBy Self).
func TestTheFallenPingsOnlyThePlayersItHasDamaged(t *testing.T) {
	t.Parallel()

	g, p, other := newTwoPlayerGameOn(t, scenarioDB(t))
	libraryCards(t, g, p, 8)
	libraryCards(t, g, other, 8)
	g.SetTurnState(1, p, engine.Main1)
	fallen := g.NewCard(corpusCard(t, "The Fallen"), p, engine.Battlefield)
	g.Card(fallen).SummonSick = false
	c := engine.NewScriptedController()
	toStep(t, g, c, p, engine.Upkeep, nil) // nothing damaged yet
	if got := g.Player(other).Life; got != 20 {
		t.Fatalf("life before any damage = %d, want 20", got)
	}
	g.SetTurnState(3, p, engine.DeclareAttackers)
	ac := engine.NewScriptedController()
	ac.QueueAttackers([]engine.CardID{fallen})
	declareAttackers(t, g, ac)
	bc := engine.NewScriptedController()
	bc.QueueBlocks(nil)
	declareBlockers(t, g, bc)
	g.DealCombatDamage(engine.NewScriptedController())
	if got := g.Player(other).Life; got != 18 {
		t.Fatalf("life after combat = %d, want 18", got)
	}
	toStep(t, g, c, p, engine.Upkeep, nil)
	if got := g.Player(other).Life; got != 17 {
		t.Errorf("life after the next upkeep = %d, want 17", got)
	}
}
