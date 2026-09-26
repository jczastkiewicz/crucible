package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
)

// TestFogEffectPreventsCombatDamageOnly proves Fog: combat damage is
// prevented for the rest of the turn, non-combat damage is not.
func TestFogEffectPreventsCombatDamageOnly(t *testing.T) {
	t.Parallel()

	g, p, other := newTwoPlayerGame(t)
	attacker := g.NewCard(creatureDefPT(t, "3", "3"), p, engine.Battlefield)
	c := engine.NewScriptedController()
	def := etbChainDef(t, "Test Fog", "DB$ Fog | SubAbility$ DBBurn",
		"DBBurn", "DB$ DealDamage | Defined$ Opponent | NumDmg$ 2")
	if _, err := castETBChain(t, g, p, def, c); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if got := g.Player(other).Life; got != 18 {
		t.Fatalf("life after burn = %d, want 18 (non-combat damage not prevented)", got)
	}
	ac := engine.NewScriptedController()
	ac.QueueAttackers([]engine.CardID{attacker})
	declareAttackers(t, g, ac)
	bc := engine.NewScriptedController()
	bc.QueueBlocks(nil)
	declareBlockers(t, g, bc)
	g.DealCombatDamage(engine.NewScriptedController())
	if got := g.Player(other).Life; got != 18 {
		t.Errorf("life after combat = %d, want 18 (combat damage prevented)", got)
	}
}
