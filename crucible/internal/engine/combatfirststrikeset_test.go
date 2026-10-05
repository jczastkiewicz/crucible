package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
)

// firstStrikeSetGame has attacker (power 2) unblocked against the other
// player, who starts at 20.
func firstStrikeSetGame(t *testing.T, keywords ...string) (g *engine.Game, defender engine.PlayerID, attacker engine.CardID) {
	t.Helper()
	g = newGame(t, "a", "b")
	p := g.Players()[0]
	defender = g.Players()[1]
	g.Player(defender).Life = 20
	g.SetTurnState(1, p, engine.Main1)
	attacker = g.NewCard(creatureDefPTKeywords(t, "2", "2", keywords...), p, engine.Battlefield)
	ac := engine.NewScriptedController()
	ac.QueueAttackers([]engine.CardID{attacker})
	declareAttackers(t, g, ac)
	declareBlockers(t, g, engine.NewScriptedController())
	return g, defender, attacker
}

// TestCombatantThatDealtFirstStrikeDamageSitsOutTheRegularStep pins
// Combat.dealDamageThisPhase (Combat.java:906-917): the regular step skips
// whoever took part in the first-strike step, even one that has lost first
// strike since -- reading the keyword at the regular step would deal damage
// a second time.
func TestCombatantThatDealtFirstStrikeDamageSitsOutTheRegularStep(t *testing.T) {
	t.Parallel()

	g, defender, attacker := firstStrikeSetGame(t, "First Strike")
	c := engine.NewScriptedController()
	g.DealFirstStrikeDamage(c)
	g.Card(attacker).KeywordMod.Add(engine.KeywordEffect{RemoveKeywords: []string{"First Strike"}})
	g.DealCombatDamage(c)
	if got := g.Player(defender).Life; got != 18 {
		t.Errorf("defender life = %d, want 18 (one 2-point hit, in the first-strike step only)", got)
	}
}

// TestCombatantThatSkippedFirstStrikeDealsInTheRegularStepEvenWithFirstStrikeNow:
// the mirror -- a creature that did not take part in the first-strike step
// (it had no first strike then) deals damage in the regular step even if it
// has gained first strike since.
func TestCombatantThatSkippedFirstStrikeDealsInTheRegularStepEvenWithFirstStrikeNow(t *testing.T) {
	t.Parallel()

	g, defender, attacker := firstStrikeSetGame(t)
	c := engine.NewScriptedController()
	g.DealFirstStrikeDamage(c)
	if got := g.Player(defender).Life; got != 20 {
		t.Fatalf("defender life = %d after the first-strike step, want 20", got)
	}
	g.Card(attacker).KeywordMod.Add(engine.KeywordEffect{AddKeywords: []string{"First Strike"}})
	g.DealCombatDamage(c)
	if got := g.Player(defender).Life; got != 18 {
		t.Errorf("defender life = %d, want 18 (the regular step deals damage to anyone who has not yet)", got)
	}
}

// TestDoubleStrikeDealsInBothSteps keeps CR 702.4b: the set never stops a
// double striker.
func TestDoubleStrikeDealsInBothSteps(t *testing.T) {
	t.Parallel()

	g, defender, _ := firstStrikeSetGame(t, "Double Strike")
	c := engine.NewScriptedController()
	g.DealFirstStrikeDamage(c)
	g.DealCombatDamage(c)
	if got := g.Player(defender).Life; got != 16 {
		t.Errorf("defender life = %d, want 16", got)
	}
}
