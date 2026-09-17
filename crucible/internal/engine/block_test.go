package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
)

// An untapped defending creature is offered and, once declared, blocks --
// CR 509.1, the ordinary case.
func TestDeclareCombatBlockersAssignsADeclaredBlocker(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	a, b := g.Players()[0], g.Players()[1]
	g.SetTurnState(1, a, engine.Main1)
	attacker := g.NewCard(creatureDefPT(t, "2", "2"), a, engine.Battlefield)
	blocker := g.NewCard(creatureDefPT(t, "2", "2"), b, engine.Battlefield)

	ac := engine.NewScriptedController()
	ac.QueueAttackers([]engine.CardID{attacker})
	g.DeclareCombatAttackers(ac)

	bc := engine.NewScriptedController()
	bc.QueueBlocks([]engine.Block{{Blocker: blocker, Attacker: attacker}})
	got := g.DeclareCombatBlockers(bc)

	want := []engine.Block{{Blocker: blocker, Attacker: attacker}}
	if len(got) != 1 || got[0] != want[0] {
		t.Fatalf("DeclareCombatBlockers() = %v, want %v", got, want)
	}
	blocks := g.Blocks()
	if len(blocks) != 1 || blocks[0] != want[0] {
		t.Errorf("Blocks() = %v, want %v", blocks, want)
	}
}

// Blocking does not tap the blocker -- CR 509 has no equivalent of CR
// 508.1f.
func TestDeclareCombatBlockersDoesNotTapTheBlocker(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	a, b := g.Players()[0], g.Players()[1]
	g.SetTurnState(1, a, engine.Main1)
	attacker := g.NewCard(creatureDefPT(t, "2", "2"), a, engine.Battlefield)
	blocker := g.NewCard(creatureDefPT(t, "2", "2"), b, engine.Battlefield)

	ac := engine.NewScriptedController()
	ac.QueueAttackers([]engine.CardID{attacker})
	g.DeclareCombatAttackers(ac)

	bc := engine.NewScriptedController()
	bc.QueueBlocks([]engine.Block{{Blocker: blocker, Attacker: attacker}})
	g.DeclareCombatBlockers(bc)

	if g.Card(blocker).Tapped {
		t.Error("a declared blocker tapped")
	}
}

// A tapped creature is never eligible to block.
func TestDeclareCombatBlockersTappedIsIneligible(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	a, b := g.Players()[0], g.Players()[1]
	g.SetTurnState(1, a, engine.Main1)
	attacker := g.NewCard(creatureDefPT(t, "2", "2"), a, engine.Battlefield)
	blocker := g.NewCard(creatureDefPT(t, "2", "2"), b, engine.Battlefield)
	g.Card(blocker).Tapped = true

	ac := engine.NewScriptedController()
	ac.QueueAttackers([]engine.CardID{attacker})
	g.DeclareCombatAttackers(ac)

	// No QueueBlocks call: if DeclareCombatBlockers asked anyway, this
	// panics on an empty queue, which is exactly the assertion -- nothing
	// eligible means nothing asked.
	bc := engine.NewScriptedController()
	got := g.DeclareCombatBlockers(bc)

	if got != nil {
		t.Errorf("DeclareCombatBlockers() = %v, want nil", got)
	}
}

// A creature controlled by the attacking player is never offered as a
// blocker, even if it would otherwise be eligible.
func TestDeclareCombatBlockersOnlyOffersDefendingPlayersCreatures(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	a := g.Players()[0]
	g.SetTurnState(1, a, engine.Main1)
	attacker := g.NewCard(creatureDefPT(t, "2", "2"), a, engine.Battlefield)
	g.NewCard(creatureDefPT(t, "2", "2"), a, engine.Battlefield) // another of the attacker's own creatures

	ac := engine.NewScriptedController()
	ac.QueueAttackers([]engine.CardID{attacker})
	g.DeclareCombatAttackers(ac)

	bc := engine.NewScriptedController()
	got := g.DeclareCombatBlockers(bc)

	if got != nil {
		t.Errorf("DeclareCombatBlockers() = %v, want nil (the only untapped creature belongs to the attacking player)", got)
	}
}

// No attackers means the defending player is never asked at all -- an empty
// queue must not panic.
func TestDeclareCombatBlockersAsksNoOneWithNoAttackers(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	a, b := g.Players()[0], g.Players()[1]
	g.SetTurnState(1, a, engine.Main1)
	g.NewCard(creatureDefPT(t, "2", "2"), b, engine.Battlefield)

	c := engine.NewScriptedController()
	got := g.DeclareCombatBlockers(c)

	if got != nil {
		t.Errorf("DeclareCombatBlockers() = %v, want nil", got)
	}
}

// Declining to block anything is itself a legal, queueable answer.
func TestDeclareCombatBlockersCanDeclineWithEligibleCreatures(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	a, b := g.Players()[0], g.Players()[1]
	g.SetTurnState(1, a, engine.Main1)
	attacker := g.NewCard(creatureDefPT(t, "2", "2"), a, engine.Battlefield)
	g.NewCard(creatureDefPT(t, "2", "2"), b, engine.Battlefield)

	ac := engine.NewScriptedController()
	ac.QueueAttackers([]engine.CardID{attacker})
	g.DeclareCombatAttackers(ac)

	bc := engine.NewScriptedController()
	bc.QueueBlocks(nil)
	got := g.DeclareCombatBlockers(bc)

	if got != nil {
		t.Errorf("DeclareCombatBlockers() = %v, want nil", got)
	}
}

// Gang blocking is legal: one attacker can receive more than one Block (CR
// 509.1c only limits the blocker side).
func TestDeclareCombatBlockersAllowsGangBlocking(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	a, b := g.Players()[0], g.Players()[1]
	g.SetTurnState(1, a, engine.Main1)
	attacker := g.NewCard(creatureDefPT(t, "4", "4"), a, engine.Battlefield)
	blocker1 := g.NewCard(creatureDefPT(t, "2", "2"), b, engine.Battlefield)
	blocker2 := g.NewCard(creatureDefPT(t, "2", "2"), b, engine.Battlefield)

	ac := engine.NewScriptedController()
	ac.QueueAttackers([]engine.CardID{attacker})
	g.DeclareCombatAttackers(ac)

	bc := engine.NewScriptedController()
	bc.QueueBlocks([]engine.Block{
		{Blocker: blocker1, Attacker: attacker},
		{Blocker: blocker2, Attacker: attacker},
	})
	got := g.DeclareCombatBlockers(bc)

	if len(got) != 2 {
		t.Fatalf("DeclareCombatBlockers() = %v, want 2 blocks", got)
	}
}

// A combat split across two defending players at once (CR 506.4) asks each
// defender separately, offering only their own creatures against only the
// attacker(s) actually attacking them.
func TestDeclareCombatBlockersSplitAcrossTwoDefendingPlayers(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b", "c")
	a, b, c2 := g.Players()[0], g.Players()[1], g.Players()[2]
	g.SetTurnState(1, a, engine.Main1)
	attackerOfB := g.NewCard(creatureDefPT(t, "2", "2"), a, engine.Battlefield)
	attackerOfC := g.NewCard(creatureDefPT(t, "2", "2"), a, engine.Battlefield)
	blockerB := g.NewCard(creatureDefPT(t, "2", "2"), b, engine.Battlefield)
	blockerC := g.NewCard(creatureDefPT(t, "2", "2"), c2, engine.Battlefield)

	ac := engine.NewScriptedController()
	ac.QueueAttackers([]engine.CardID{attackerOfB, attackerOfC})
	ac.QueueAttackTarget(engine.PlayerEntity(b))
	ac.QueueAttackTarget(engine.PlayerEntity(c2))
	g.DeclareCombatAttackers(ac)

	// attackerOfB is first in the attackers slice, so b is asked first;
	// c2's answer is queued second.
	bc := engine.NewScriptedController()
	bc.QueueBlocks([]engine.Block{{Blocker: blockerB, Attacker: attackerOfB}})
	bc.QueueBlocks([]engine.Block{{Blocker: blockerC, Attacker: attackerOfC}})

	got := g.DeclareCombatBlockers(bc)

	want := []engine.Block{
		{Blocker: blockerB, Attacker: attackerOfB},
		{Blocker: blockerC, Attacker: attackerOfC},
	}
	if len(got) != len(want) {
		t.Fatalf("DeclareCombatBlockers() = %v, want %v", got, want)
	}
	for i, b := range want {
		if got[i] != b {
			t.Errorf("DeclareCombatBlockers()[%d] = %v, want %v", i, got[i], b)
		}
	}
}

// A defender with nothing eligible to block with is skipped, not asked with
// an empty list -- the other defender's block still goes through.
func TestDeclareCombatBlockersSkipsADefenderWithNoEligibleCreature(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b", "c")
	a, b, c2 := g.Players()[0], g.Players()[1], g.Players()[2]
	g.SetTurnState(1, a, engine.Main1)
	attackerOfB := g.NewCard(creatureDefPT(t, "2", "2"), a, engine.Battlefield)
	attackerOfC := g.NewCard(creatureDefPT(t, "2", "2"), a, engine.Battlefield)
	blockerC := g.NewCard(creatureDefPT(t, "2", "2"), c2, engine.Battlefield)
	// b has no creature at all.

	ac := engine.NewScriptedController()
	ac.QueueAttackers([]engine.CardID{attackerOfB, attackerOfC})
	ac.QueueAttackTarget(engine.PlayerEntity(b))
	ac.QueueAttackTarget(engine.PlayerEntity(c2))
	g.DeclareCombatAttackers(ac)

	bc := engine.NewScriptedController()
	bc.QueueBlocks([]engine.Block{{Blocker: blockerC, Attacker: attackerOfC}})

	got := g.DeclareCombatBlockers(bc)

	want := []engine.Block{{Blocker: blockerC, Attacker: attackerOfC}}
	if len(got) != 1 || got[0] != want[0] {
		t.Errorf("DeclareCombatBlockers() = %v, want %v", got, want)
	}
}

// A Menace attacker (CR 702.111b) blocked by only one creature has the
// whole illegal block dropped, not reduced to a single-blocker assignment.
func TestDeclareCombatBlockersDropsSingleBlockerAgainstMenace(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	a, b := g.Players()[0], g.Players()[1]
	g.SetTurnState(1, a, engine.Main1)
	attacker := g.NewCard(creatureDefPTKeywords(t, "2", "2", "Menace"), a, engine.Battlefield)
	blocker := g.NewCard(creatureDefPT(t, "2", "2"), b, engine.Battlefield)

	ac := engine.NewScriptedController()
	ac.QueueAttackers([]engine.CardID{attacker})
	g.DeclareCombatAttackers(ac)

	bc := engine.NewScriptedController()
	bc.QueueBlocks([]engine.Block{{Blocker: blocker, Attacker: attacker}})
	got := g.DeclareCombatBlockers(bc)

	if got != nil {
		t.Errorf("DeclareCombatBlockers() = %v, want nil -- one creature can't legally block a Menace attacker", got)
	}
	if len(g.Blocks()) != 0 {
		t.Errorf("Blocks() = %v, want none", g.Blocks())
	}
}

// A Menace attacker blocked by two creatures is legal -- CR 702.111b's own
// requirement met exactly.
func TestDeclareCombatBlockersAllowsTwoBlockersAgainstMenace(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	a, b := g.Players()[0], g.Players()[1]
	g.SetTurnState(1, a, engine.Main1)
	attacker := g.NewCard(creatureDefPTKeywords(t, "2", "2", "Menace"), a, engine.Battlefield)
	blocker1 := g.NewCard(creatureDefPT(t, "2", "2"), b, engine.Battlefield)
	blocker2 := g.NewCard(creatureDefPT(t, "2", "2"), b, engine.Battlefield)

	ac := engine.NewScriptedController()
	ac.QueueAttackers([]engine.CardID{attacker})
	g.DeclareCombatAttackers(ac)

	bc := engine.NewScriptedController()
	bc.QueueBlocks([]engine.Block{
		{Blocker: blocker1, Attacker: attacker},
		{Blocker: blocker2, Attacker: attacker},
	})
	got := g.DeclareCombatBlockers(bc)

	if len(got) != 2 {
		t.Fatalf("DeclareCombatBlockers() = %v, want 2 blocks", got)
	}
}

// A cloned game's combat state is its own slice: declaring on the clone
// must not write back to the original.
func TestCloneCopiesBlocks(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	a, b := g.Players()[0], g.Players()[1]
	g.SetTurnState(1, a, engine.Main1)
	attacker := g.NewCard(creatureDefPT(t, "2", "2"), a, engine.Battlefield)
	blocker := g.NewCard(creatureDefPT(t, "2", "2"), b, engine.Battlefield)

	ac := engine.NewScriptedController()
	ac.QueueAttackers([]engine.CardID{attacker})
	g.DeclareCombatAttackers(ac)

	clone := g.Clone()
	bc := engine.NewScriptedController()
	bc.QueueBlocks([]engine.Block{{Blocker: blocker, Attacker: attacker}})
	clone.DeclareCombatBlockers(bc)

	if len(g.Blocks()) != 0 {
		t.Errorf("original Blocks() = %v after the clone's changed, want none", g.Blocks())
	}
	if len(clone.Blocks()) != 1 {
		t.Errorf("clone Blocks() = %v, want one", clone.Blocks())
	}
}
