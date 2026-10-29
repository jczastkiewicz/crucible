package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
)

// Mode$ PayEcho and Mode$ PayCumulativeUpkeep fire from the Echo$ /
// CumulativeUpkeep$ Sacrifice ability once the player answered
// (SacrificeEffect.java:48,71): Paid$ True when the {0} cost was paid. The
// ability stands in for the upkeep trigger the keyword expands to.
func TestPayEchoTriggerFiresWithThePaidAnswer(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGame(t)
	c := engine.NewScriptedController()
	c.QueueConfirmPayCost(true)
	host := g.NewCard(copyTestDef(t, "Efreet", "Creature Giant", "6", "6",
		"A:AB$ Sacrifice | Cost$ 0 | Defined$ Self | SacValid$ Self | Echo$ 0",
		"T:Mode$ PayEcho | ValidCard$ Card.Self | Paid$ True | TriggerZones$ Battlefield | Execute$ TrigGain",
		"SVar:TrigGain:DB$ GainLife | Defined$ You | LifeAmount$ 3"), p, engine.Battlefield)
	start := g.Player(p).Life
	if !g.ActivateAbility(p, host, 0, c) {
		t.Fatalf("ActivateAbility failed: %v", g.TakePendingError())
	}
	if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if got := g.Player(p).Life - start; got != 3 {
		t.Errorf("life gained = %d, want 3", got)
	}
	if got := g.Card(host).Zone; got != engine.Battlefield {
		t.Errorf("host zone = %v, want Battlefield: the echo cost was paid", got)
	}
}
