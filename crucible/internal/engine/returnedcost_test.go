package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
)

// TestActivateAbilityAttachedByReturnedNamesWhatTheReturnedCardWasAttachedTo
// proves Defined$ AttachedBy Returned (Krasis Incubation): the card a Return
// cost sent to hand still names the permanent it was attached to, read from
// its last-known information (AbilityUtils.java:109-116, CardCopyService.java:75).
func TestActivateAbilityAttachedByReturnedNamesWhatTheReturnedCardWasAttachedTo(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGame(t)
	host := g.NewCard(creatureDefWithAbility(t, "Returner",
		"AB$ PutCounter | Cost$ Return<1/CARDNAME> | Defined$ AttachedBy Returned | CounterType$ P1P1 | CounterNum$ 1"), p, engine.Battlefield)
	target := g.NewCard(creatureDefWithAbility(t, "Bystander", "AB$ Draw | Cost$ T"), p, engine.Battlefield)
	g.Attach(host, target)

	c := engine.NewScriptedController()
	if !g.ActivateAbility(p, host, 0, c) {
		t.Fatal("ActivateAbility failed")
	}
	if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if got := g.Card(host).Zone; got != engine.Hand {
		t.Errorf("host zone = %v, want Hand", got)
	}
	if got := g.Card(target).Counters.Count(engine.P1P1); got != 1 {
		t.Errorf("counters on the formerly enchanted creature = %d, want 1", got)
	}
}
