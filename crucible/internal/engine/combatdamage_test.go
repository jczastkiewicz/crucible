package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
)

// An unblocked attacker deals its power to the defending player -- CR
// 510.1a.
func TestDealCombatDamageUnblockedAttackerHitsThePlayer(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	a, b := g.Players()[0], g.Players()[1]
	g.Player(b).Life = 20
	g.SetTurnState(1, a, engine.Main1)
	attacker := g.NewCard(creatureDefPT(t, "3", "3"), a, engine.Battlefield)

	ac := engine.NewScriptedController()
	ac.QueueAttackers([]engine.CardID{attacker})
	g.DeclareCombatAttackers(ac)
	bc := engine.NewScriptedController()
	bc.QueueBlocks(nil)
	g.DeclareCombatBlockers(bc)

	var sink recordingSink
	g.SetSink(&sink)
	g.DealCombatDamage(engine.NewScriptedController())

	if g.Player(b).Life != 17 {
		t.Errorf("defender life = %d, want 17", g.Player(b).Life)
	}
	var sawDamage, sawLife bool
	for _, e := range sink.events {
		if e.Kind == engine.DamageDealt && e.Amount == 3 {
			sawDamage = true
		}
		if e.Kind == engine.LifeChanged && e.Amount == -3 {
			sawLife = true
		}
	}
	if !sawDamage {
		t.Error("no DamageDealt event for the unblocked attacker")
	}
	if !sawLife {
		t.Error("no LifeChanged event for the defender")
	}
}

// A single blocker exchanges full power for full power with its attacker --
// CR 510.1b, no decision needed.
func TestDealCombatDamageSingleBlockerExchangesFullPower(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	a, b := g.Players()[0], g.Players()[1]
	g.SetTurnState(1, a, engine.Main1)
	attacker := g.NewCard(creatureDefPT(t, "3", "4"), a, engine.Battlefield)
	blocker := g.NewCard(creatureDefPT(t, "2", "5"), b, engine.Battlefield)

	ac := engine.NewScriptedController()
	ac.QueueAttackers([]engine.CardID{attacker})
	g.DeclareCombatAttackers(ac)
	bc := engine.NewScriptedController()
	bc.QueueBlocks([]engine.Block{{Blocker: blocker, Attacker: attacker}})
	g.DeclareCombatBlockers(bc)

	g.DealCombatDamage(engine.NewScriptedController())

	if g.Card(blocker).Damage.Marked != 3 {
		t.Errorf("blocker damage = %d, want 3 (attacker's power)", g.Card(blocker).Damage.Marked)
	}
	if g.Card(attacker).Damage.Marked != 2 {
		t.Errorf("attacker damage = %d, want 2 (blocker's power)", g.Card(attacker).Damage.Marked)
	}
}

// A gang-blocked attacker asks the attacking player how to divide its power
// -- CR 510.1c.
func TestDealCombatDamageGangBlockAsksForAssignment(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	a, b := g.Players()[0], g.Players()[1]
	g.SetTurnState(1, a, engine.Main1)
	attacker := g.NewCard(creatureDefPT(t, "5", "5"), a, engine.Battlefield)
	blocker1 := g.NewCard(creatureDefPT(t, "1", "1"), b, engine.Battlefield)
	blocker2 := g.NewCard(creatureDefPT(t, "1", "1"), b, engine.Battlefield)

	ac := engine.NewScriptedController()
	ac.QueueAttackers([]engine.CardID{attacker})
	g.DeclareCombatAttackers(ac)
	bc := engine.NewScriptedController()
	bc.QueueBlocks([]engine.Block{
		{Blocker: blocker1, Attacker: attacker},
		{Blocker: blocker2, Attacker: attacker},
	})
	g.DeclareCombatBlockers(bc)

	dc := engine.NewScriptedController()
	dc.QueueDamageAssignment([]engine.DamageAssignment{
		{Blocker: blocker1, Amount: 1},
		{Blocker: blocker2, Amount: 4},
	})
	g.DealCombatDamage(dc)

	if g.Card(blocker1).Damage.Marked != 1 {
		t.Errorf("blocker1 damage = %d, want 1", g.Card(blocker1).Damage.Marked)
	}
	if g.Card(blocker2).Damage.Marked != 4 {
		t.Errorf("blocker2 damage = %d, want 4", g.Card(blocker2).Damage.Marked)
	}
	// Both blockers still deal their own full power back to the attacker.
	if g.Card(attacker).Damage.Marked != 2 {
		t.Errorf("attacker damage = %d, want 2 (1+1 from both blockers)", g.Card(attacker).Damage.Marked)
	}
}

// Deathtouch makes any nonzero damage lethal, recorded as a flag on the
// target, not as a reduced amount -- CR 702.2b/704.5g.
func TestDealCombatDamageDeathtouchSetsTheFlag(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	a, b := g.Players()[0], g.Players()[1]
	g.SetTurnState(1, a, engine.Main1)
	attacker := g.NewCard(creatureDefPTKeywords(t, "1", "1", "Deathtouch"), a, engine.Battlefield)
	blocker := g.NewCard(creatureDefPT(t, "9", "9"), b, engine.Battlefield)

	ac := engine.NewScriptedController()
	ac.QueueAttackers([]engine.CardID{attacker})
	g.DeclareCombatAttackers(ac)
	bc := engine.NewScriptedController()
	bc.QueueBlocks([]engine.Block{{Blocker: blocker, Attacker: attacker}})
	g.DeclareCombatBlockers(bc)

	g.DealCombatDamage(engine.NewScriptedController())

	if !g.Card(blocker).Damage.Deathtouch {
		t.Error("blocker's damage is not flagged deathtouch")
	}
	// The non-deathtouch blocker's own damage back does not set the flag.
	if g.Card(attacker).Damage.Deathtouch {
		t.Error("attacker's damage is flagged deathtouch, but the blocker doesn't have it")
	}
}

// A zero-power attacker or blocker deals no damage and, for an attacker,
// never reaches for a defending player at all.
func TestDealCombatDamageZeroPowerDealsNothing(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	a, b := g.Players()[0], g.Players()[1]
	g.Player(b).Life = 20
	g.SetTurnState(1, a, engine.Main1)
	attacker := g.NewCard(creatureDefPT(t, "0", "3"), a, engine.Battlefield)

	ac := engine.NewScriptedController()
	ac.QueueAttackers([]engine.CardID{attacker})
	g.DeclareCombatAttackers(ac)
	bc := engine.NewScriptedController()
	bc.QueueBlocks(nil)
	g.DeclareCombatBlockers(bc)

	// No sink events expected and no controller call needed either way; an
	// AssignCombatDamage call here would panic on the always-empty queue,
	// which would fail the test if the zero-power short-circuit didn't hold.
	g.DealCombatDamage(engine.NewScriptedController())

	if g.Player(b).Life != 20 {
		t.Errorf("defender life = %d, want unchanged at 20", g.Player(b).Life)
	}
}

// No attackers means DealCombatDamage does nothing -- it must not panic
// walking an empty Attackers slice.
func TestDealCombatDamageNoAttackersIsANoop(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	a := g.Players()[0]
	g.SetTurnState(1, a, engine.Main1)

	g.DealCombatDamage(engine.NewScriptedController())
}
