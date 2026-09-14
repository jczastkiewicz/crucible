package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
)

// A plain untapped, non-summoning-sick creature is eligible and, once
// declared, taps -- CR 508.1a/508.1f, the ordinary case.
func TestDeclareCombatAttackersTapsADeclaredAttacker(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	a := g.Players()[0]
	g.SetTurnState(1, a, engine.Main1)
	creature := g.NewCard(creatureDefPT(t, "2", "2"), a, engine.Battlefield)

	c := engine.NewScriptedController()
	c.QueueAttackers([]engine.CardID{creature})

	got := g.DeclareCombatAttackers(c)

	if len(got) != 1 || got[0] != creature {
		t.Fatalf("DeclareCombatAttackers() = %v, want [%v]", got, creature)
	}
	if !g.Card(creature).Tapped {
		t.Error("declared attacker did not tap")
	}
	attackers := g.Attackers()
	if len(attackers) != 1 || attackers[0] != creature {
		t.Errorf("Attackers() = %v, want [%v]", attackers, creature)
	}
}

// Vigilance keeps a declared attacker untapped (CR 508.1f).
func TestDeclareCombatAttackersVigilanceStaysUntapped(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	a := g.Players()[0]
	g.SetTurnState(1, a, engine.Main1)
	creature := g.NewCard(creatureDefPTKeywords(t, "2", "2", "Vigilance"), a, engine.Battlefield)

	c := engine.NewScriptedController()
	c.QueueAttackers([]engine.CardID{creature})
	g.DeclareCombatAttackers(c)

	if g.Card(creature).Tapped {
		t.Error("a vigilance attacker tapped")
	}
}

// A summoning-sick creature is not eligible unless it has haste (CR 302.6,
// 508.1a).
func TestDeclareCombatAttackersSummoningSickIsIneligibleWithoutHaste(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	a := g.Players()[0]
	g.SetTurnState(1, a, engine.Main1)
	sick := g.NewCard(creatureDefPT(t, "2", "2"), a, engine.Battlefield)
	g.Card(sick).SummonSick = true
	haste := g.NewCard(creatureDefPTKeywords(t, "2", "2", "Haste"), a, engine.Battlefield)
	g.Card(haste).SummonSick = true

	c := engine.NewScriptedController()
	c.QueueAttackers([]engine.CardID{haste})
	got := g.DeclareCombatAttackers(c)

	if len(got) != 1 || got[0] != haste {
		t.Errorf("DeclareCombatAttackers() = %v, want [%v] (only the hasty one was ever offered)", got, haste)
	}
}

// A tapped creature is never eligible, haste or not.
func TestDeclareCombatAttackersTappedIsIneligible(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	a := g.Players()[0]
	g.SetTurnState(1, a, engine.Main1)
	tapped := g.NewCard(creatureDefPT(t, "2", "2"), a, engine.Battlefield)
	g.Card(tapped).Tapped = true

	// No QueueAttackers call: if DeclareCombatAttackers asked anyway, this
	// panics on an empty queue, which is exactly the assertion -- nothing
	// eligible means nothing asked.
	c := engine.NewScriptedController()
	got := g.DeclareCombatAttackers(c)

	if got != nil {
		t.Errorf("DeclareCombatAttackers() = %v, want nil", got)
	}
}

// A creature controlled by the non-active player is never offered, even if
// it would otherwise be eligible.
func TestDeclareCombatAttackersOnlyOffersActivePlayersCreatures(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	a, b := g.Players()[0], g.Players()[1]
	g.SetTurnState(1, a, engine.Main1)
	g.NewCard(creatureDefPT(t, "2", "2"), b, engine.Battlefield)

	c := engine.NewScriptedController()
	got := g.DeclareCombatAttackers(c)

	if got != nil {
		t.Errorf("DeclareCombatAttackers() = %v, want nil (the eligible creature belongs to the defending player)", got)
	}
}

// No eligible creature means the controller is never asked at all -- an
// empty queue must not panic.
func TestDeclareCombatAttackersAsksNoOneWhenNothingIsEligible(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	a := g.Players()[0]
	g.SetTurnState(1, a, engine.Main1)

	c := engine.NewScriptedController()
	got := g.DeclareCombatAttackers(c)

	if got != nil {
		t.Errorf("DeclareCombatAttackers() = %v, want nil", got)
	}
}

// Declining to attack with anything is itself a legal, queueable answer.
func TestDeclareCombatAttackersCanDeclineWithEligibleCreatures(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	a := g.Players()[0]
	g.SetTurnState(1, a, engine.Main1)
	creature := g.NewCard(creatureDefPT(t, "2", "2"), a, engine.Battlefield)

	c := engine.NewScriptedController()
	c.QueueAttackers(nil)
	got := g.DeclareCombatAttackers(c)

	if got != nil {
		t.Errorf("DeclareCombatAttackers() = %v, want nil", got)
	}
	if g.Card(creature).Tapped {
		t.Error("a creature that was not declared as an attacker tapped anyway")
	}
}

// A cloned game's combat state is its own slice: declaring on the clone
// must not write back to the original.
func TestCloneCopiesCombat(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	a := g.Players()[0]
	g.SetTurnState(1, a, engine.Main1)
	creature := g.NewCard(creatureDefPT(t, "2", "2"), a, engine.Battlefield)

	clone := g.Clone()
	c := engine.NewScriptedController()
	c.QueueAttackers([]engine.CardID{creature})
	clone.DeclareCombatAttackers(c)

	if len(g.Attackers()) != 0 {
		t.Errorf("original Attackers() = %v after the clone's changed, want none", g.Attackers())
	}
	if len(clone.Attackers()) != 1 {
		t.Errorf("clone Attackers() = %v, want one", clone.Attackers())
	}
}
