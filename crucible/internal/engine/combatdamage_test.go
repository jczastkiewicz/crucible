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
	declareAttackers(t, g, ac)
	bc := engine.NewScriptedController()
	bc.QueueBlocks(nil)
	declareBlockers(t, g, bc)

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
	declareAttackers(t, g, ac)
	bc := engine.NewScriptedController()
	bc.QueueBlocks([]engine.Block{{Blocker: blocker, Attacker: attacker}})
	declareBlockers(t, g, bc)

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
	declareAttackers(t, g, ac)
	bc := engine.NewScriptedController()
	bc.QueueBlocks([]engine.Block{
		{Blocker: blocker1, Attacker: attacker},
		{Blocker: blocker2, Attacker: attacker},
	})
	declareBlockers(t, g, bc)

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
	declareAttackers(t, g, ac)
	bc := engine.NewScriptedController()
	bc.QueueBlocks([]engine.Block{{Blocker: blocker, Attacker: attacker}})
	declareBlockers(t, g, bc)

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
	declareAttackers(t, g, ac)
	bc := engine.NewScriptedController()
	bc.QueueBlocks(nil)
	declareBlockers(t, g, bc)

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

// declareOneAttackerAndBlocker is the common setup every first-strike/trample
// test starts from: a attacking, blocker blocking, nothing dealt yet.
func declareOneAttackerAndBlocker(t *testing.T, g *engine.Game, attacker, blocker engine.CardID) {
	t.Helper()
	ac := engine.NewScriptedController()
	ac.QueueAttackers([]engine.CardID{attacker})
	declareAttackers(t, g, ac)
	bc := engine.NewScriptedController()
	bc.QueueBlocks([]engine.Block{{Blocker: blocker, Attacker: attacker}})
	declareBlockers(t, g, bc)
}

// In the first-strike step, only a first striker deals damage -- its
// non-first-strike blocker does not hit back yet (CR 510.4).
func TestDealFirstStrikeDamageOnlyFirstStrikersAct(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	a, b := g.Players()[0], g.Players()[1]
	g.SetTurnState(1, a, engine.Main1)
	attacker := g.NewCard(creatureDefPTKeywords(t, "3", "3", "First Strike"), a, engine.Battlefield)
	blocker := g.NewCard(creatureDefPT(t, "2", "9"), b, engine.Battlefield)
	declareOneAttackerAndBlocker(t, g, attacker, blocker)

	g.DealFirstStrikeDamage(engine.NewScriptedController())

	if g.Card(blocker).Damage.Marked != 3 {
		t.Errorf("blocker damage = %d, want 3 (the first striker's power)", g.Card(blocker).Damage.Marked)
	}
	if g.Card(attacker).Damage.Marked != 0 {
		t.Errorf("attacker damage = %d, want 0 (its blocker has no first strike yet)", g.Card(attacker).Damage.Marked)
	}
}

// A creature with only first strike does not deal damage again in the
// regular step; its non-first-strike blocker, which sat out the first
// strike step, hits back here instead.
func TestDealCombatDamageOnlyFirstStrikeDoesNotActAgain(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	a, b := g.Players()[0], g.Players()[1]
	g.SetTurnState(1, a, engine.Main1)
	attacker := g.NewCard(creatureDefPTKeywords(t, "3", "3", "First Strike"), a, engine.Battlefield)
	blocker := g.NewCard(creatureDefPT(t, "2", "9"), b, engine.Battlefield)
	declareOneAttackerAndBlocker(t, g, attacker, blocker)

	g.DealFirstStrikeDamage(engine.NewScriptedController())
	g.DealCombatDamage(engine.NewScriptedController())

	if g.Card(blocker).Damage.Marked != 3 {
		t.Errorf("blocker damage = %d, want 3 (unchanged: the first striker doesn't act twice)", g.Card(blocker).Damage.Marked)
	}
	if g.Card(attacker).Damage.Marked != 2 {
		t.Errorf("attacker damage = %d, want 2 (its blocker finally hits back)", g.Card(attacker).Damage.Marked)
	}
}

// Double strike acts in both steps -- CR 702.4b.
func TestDealCombatDamageDoubleStrikeActsInBothSteps(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	a, b := g.Players()[0], g.Players()[1]
	g.SetTurnState(1, a, engine.Main1)
	attacker := g.NewCard(creatureDefPTKeywords(t, "3", "3", "Double Strike"), a, engine.Battlefield)
	blocker := g.NewCard(creatureDefPT(t, "2", "9"), b, engine.Battlefield)
	declareOneAttackerAndBlocker(t, g, attacker, blocker)

	g.DealFirstStrikeDamage(engine.NewScriptedController())
	g.DealCombatDamage(engine.NewScriptedController())

	if g.Card(blocker).Damage.Marked != 6 {
		t.Errorf("blocker damage = %d, want 6 (3 from each step)", g.Card(blocker).Damage.Marked)
	}
}

// With nothing in combat carrying first strike or double strike, the first
// strike step is a safe no-op -- no controller call, no damage.
func TestDealFirstStrikeDamageWithNoFirstStrikersIsANoop(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	a, b := g.Players()[0], g.Players()[1]
	g.SetTurnState(1, a, engine.Main1)
	attacker := g.NewCard(creatureDefPT(t, "3", "3"), a, engine.Battlefield)
	blocker := g.NewCard(creatureDefPT(t, "2", "9"), b, engine.Battlefield)
	declareOneAttackerAndBlocker(t, g, attacker, blocker)

	g.DealFirstStrikeDamage(engine.NewScriptedController())

	if g.Card(blocker).Damage.Marked != 0 || g.Card(attacker).Damage.Marked != 0 {
		t.Error("the first strike step dealt damage with nobody eligible to deal it")
	}
}

// A single-blocker trampler assigns only lethal to the blocker and the rest
// to the defending player -- CR 702.19b.
func TestDealCombatDamageTrampleSingleBlockerAssignsExcessToPlayer(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	a, b := g.Players()[0], g.Players()[1]
	g.Player(b).Life = 20
	g.SetTurnState(1, a, engine.Main1)
	attacker := g.NewCard(creatureDefPTKeywords(t, "6", "6", "Trample"), a, engine.Battlefield)
	blocker := g.NewCard(creatureDefPT(t, "1", "2"), b, engine.Battlefield)
	declareOneAttackerAndBlocker(t, g, attacker, blocker)

	g.DealCombatDamage(engine.NewScriptedController())

	if g.Card(blocker).Damage.Marked != 2 {
		t.Errorf("blocker damage = %d, want 2 (exactly lethal)", g.Card(blocker).Damage.Marked)
	}
	if g.Player(b).Life != 16 {
		t.Errorf("defender life = %d, want 16 (20 - 4 trample excess)", g.Player(b).Life)
	}
}

// Deathtouch plus trample needs only 1 assigned as lethal -- CR 702.2c
// combined with 702.19b.
func TestDealCombatDamageTrampleWithDeathtouchNeedsOnlyOneLethal(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	a, b := g.Players()[0], g.Players()[1]
	g.Player(b).Life = 20
	g.SetTurnState(1, a, engine.Main1)
	attacker := g.NewCard(creatureDefPTKeywords(t, "6", "6", "Trample", "Deathtouch"), a, engine.Battlefield)
	blocker := g.NewCard(creatureDefPT(t, "1", "9"), b, engine.Battlefield)
	declareOneAttackerAndBlocker(t, g, attacker, blocker)

	g.DealCombatDamage(engine.NewScriptedController())

	if g.Card(blocker).Damage.Marked != 1 {
		t.Errorf("blocker damage = %d, want 1 (deathtouch's lethal)", g.Card(blocker).Damage.Marked)
	}
	if g.Player(b).Life != 15 {
		t.Errorf("defender life = %d, want 15 (20 - 5 trample excess)", g.Player(b).Life)
	}
}

// A gang-blocked trampler tramples whatever the controller's own assignment
// left unassigned across the named blockers -- CR 702.19c.
func TestDealCombatDamageTrampleGangBlockAssignsLeftoverToPlayer(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	a, b := g.Players()[0], g.Players()[1]
	g.Player(b).Life = 20
	g.SetTurnState(1, a, engine.Main1)
	attacker := g.NewCard(creatureDefPTKeywords(t, "10", "10", "Trample"), a, engine.Battlefield)
	blocker1 := g.NewCard(creatureDefPT(t, "1", "1"), b, engine.Battlefield)
	blocker2 := g.NewCard(creatureDefPT(t, "1", "1"), b, engine.Battlefield)

	ac := engine.NewScriptedController()
	ac.QueueAttackers([]engine.CardID{attacker})
	declareAttackers(t, g, ac)
	bc := engine.NewScriptedController()
	bc.QueueBlocks([]engine.Block{
		{Blocker: blocker1, Attacker: attacker},
		{Blocker: blocker2, Attacker: attacker},
	})
	declareBlockers(t, g, bc)

	dc := engine.NewScriptedController()
	dc.QueueDamageAssignment([]engine.DamageAssignment{
		{Blocker: blocker1, Amount: 1},
		{Blocker: blocker2, Amount: 1},
	})
	g.DealCombatDamage(dc)

	if g.Player(b).Life != 12 {
		t.Errorf("defender life = %d, want 12 (20 - 8 trample excess)", g.Player(b).Life)
	}
}

// A non-trampler's unassigned gang-block remainder is wasted, not sent to
// the player -- unchanged from before trample existed.
func TestDealCombatDamageNonTramplerWastesUnassignedDamage(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	a, b := g.Players()[0], g.Players()[1]
	g.Player(b).Life = 20
	g.SetTurnState(1, a, engine.Main1)
	attacker := g.NewCard(creatureDefPT(t, "10", "10"), a, engine.Battlefield)
	blocker1 := g.NewCard(creatureDefPT(t, "1", "1"), b, engine.Battlefield)
	blocker2 := g.NewCard(creatureDefPT(t, "1", "1"), b, engine.Battlefield)

	ac := engine.NewScriptedController()
	ac.QueueAttackers([]engine.CardID{attacker})
	declareAttackers(t, g, ac)
	bc := engine.NewScriptedController()
	bc.QueueBlocks([]engine.Block{
		{Blocker: blocker1, Attacker: attacker},
		{Blocker: blocker2, Attacker: attacker},
	})
	declareBlockers(t, g, bc)

	dc := engine.NewScriptedController()
	dc.QueueDamageAssignment([]engine.DamageAssignment{
		{Blocker: blocker1, Amount: 1},
		{Blocker: blocker2, Amount: 1},
	})
	g.DealCombatDamage(dc)

	if g.Player(b).Life != 20 {
		t.Errorf("defender life = %d, want unchanged at 20 (no trample, the rest is wasted)", g.Player(b).Life)
	}
}

// A creature killed by first-strike damage is gone by the regular step:
// dead as an attacker, it deals nothing to its (surviving) blocker, which
// therefore doesn't take a hit it wouldn't have lived to deal back either.
func TestDealCombatDamageSkipsAnAttackerKilledByFirstStrike(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	a, b := g.Players()[0], g.Players()[1]
	g.Player(a).Life, g.Player(b).Life = 20, 20
	g.SetTurnState(1, a, engine.Main1)
	attacker := g.NewCard(creatureDefPT(t, "3", "2"), a, engine.Battlefield)
	blocker := g.NewCard(creatureDefPTKeywords(t, "5", "5", "First Strike"), b, engine.Battlefield)
	declareOneAttackerAndBlocker(t, g, attacker, blocker)

	g.DealFirstStrikeDamage(engine.NewScriptedController())
	engine.CheckStateBasedActions(g, engine.NewScriptedController())

	if g.Card(attacker).Zone != engine.Graveyard {
		t.Fatalf("attacker zone = %v, want Graveyard (lethal first-strike damage)", g.Card(attacker).Zone)
	}

	// DealCombatDamage must not panic reading Power/Toughness off a card
	// that already left the battlefield, and must not mark the blocker with
	// damage from an attacker that's no longer there to deal it.
	g.DealCombatDamage(engine.NewScriptedController())

	if g.Card(blocker).Damage.Marked != 0 {
		t.Errorf("blocker damage = %d, want 0 (its attacker was already dead)", g.Card(blocker).Damage.Marked)
	}
}

// CR 510.1c: a blocked attacker whose only blocker died to first-strike
// damage remains blocked and deals nothing at all in the regular step --
// not even to the player it's attacking, since it has no trample.
func TestDealCombatDamageBlockedAttackerWithNoLiveBlockersDealsNothingWithoutTrample(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	a, b := g.Players()[0], g.Players()[1]
	g.Player(a).Life, g.Player(b).Life = 20, 20
	g.SetTurnState(1, a, engine.Main1)
	attacker := g.NewCard(creatureDefPTKeywords(t, "5", "5", "Double Strike"), a, engine.Battlefield)
	blocker := g.NewCard(creatureDefPT(t, "1", "3"), b, engine.Battlefield)
	declareOneAttackerAndBlocker(t, g, attacker, blocker)

	g.DealFirstStrikeDamage(engine.NewScriptedController())
	engine.CheckStateBasedActions(g, engine.NewScriptedController())

	if g.Card(blocker).Zone != engine.Graveyard {
		t.Fatalf("blocker zone = %v, want Graveyard (lethal first-strike damage)", g.Card(blocker).Zone)
	}

	g.DealCombatDamage(engine.NewScriptedController())

	if g.Player(b).Life != 20 {
		t.Errorf("defender life = %d, want unchanged at 20 (blocked, no trample, its blocker is gone)", g.Player(b).Life)
	}
}

// An unresolvable toughness ("*") means lethalDamage can't compute lethal
// at all -- trample falls back to "full power to the blocker," the same
// conservative default as not trampling, rather than guessing.
func TestDealCombatDamageTrampleUnresolvableToughnessAssignsFullPowerToBlocker(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	a, b := g.Players()[0], g.Players()[1]
	g.Player(b).Life = 20
	g.SetTurnState(1, a, engine.Main1)
	attacker := g.NewCard(creatureDefPTKeywords(t, "5", "5", "Trample"), a, engine.Battlefield)
	blocker := g.NewCard(creatureDefPT(t, "1", "*"), b, engine.Battlefield)
	declareOneAttackerAndBlocker(t, g, attacker, blocker)

	g.DealCombatDamage(engine.NewScriptedController())

	if g.Card(blocker).Damage.Marked != 5 {
		t.Errorf("blocker damage = %d, want 5 (full power, lethal unknowable)", g.Card(blocker).Damage.Marked)
	}
	if g.Player(b).Life != 20 {
		t.Errorf("defender life = %d, want unchanged at 20 (nothing tramples over)", g.Player(b).Life)
	}
}

// A blocker that already has damage in excess of its toughness marked
// (pre-existing, outside this combat) needs no more to stay lethal --
// lethalDamage floors at 0, not a negative "credit."
func TestDealCombatDamageTrampleAlreadyLethalBlockerNeedsNoMore(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	a, b := g.Players()[0], g.Players()[1]
	g.Player(b).Life = 20
	g.SetTurnState(1, a, engine.Main1)
	attacker := g.NewCard(creatureDefPTKeywords(t, "5", "5", "Trample"), a, engine.Battlefield)
	blocker := g.NewCard(creatureDefPT(t, "1", "2"), b, engine.Battlefield)
	g.Card(blocker).Damage.Mark(5, false)
	declareOneAttackerAndBlocker(t, g, attacker, blocker)

	g.DealCombatDamage(engine.NewScriptedController())

	if g.Card(blocker).Damage.Marked != 5 {
		t.Errorf("blocker damage = %d, want unchanged at 5 (already lethal, needs none more)", g.Card(blocker).Damage.Marked)
	}
	if g.Player(b).Life != 15 {
		t.Errorf("defender life = %d, want 15 (all 5 power tramples over)", g.Player(b).Life)
	}
}

// An AssignCombatDamage answer that assigns a blocker 0 damage is legal --
// dealCreatureDamage/dealPlayerDamage's own amount<=0 guard must not mark a
// meaningless zero or emit an event for it.
func TestDealCombatDamageZeroAmountAssignmentMarksNothing(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	a, b := g.Players()[0], g.Players()[1]
	g.SetTurnState(1, a, engine.Main1)
	attacker := g.NewCard(creatureDefPT(t, "5", "5"), a, engine.Battlefield)
	blocker1 := g.NewCard(creatureDefPT(t, "1", "1"), b, engine.Battlefield)
	blocker2 := g.NewCard(creatureDefPT(t, "1", "1"), b, engine.Battlefield)

	ac := engine.NewScriptedController()
	ac.QueueAttackers([]engine.CardID{attacker})
	declareAttackers(t, g, ac)
	bc := engine.NewScriptedController()
	bc.QueueBlocks([]engine.Block{
		{Blocker: blocker1, Attacker: attacker},
		{Blocker: blocker2, Attacker: attacker},
	})
	declareBlockers(t, g, bc)

	dc := engine.NewScriptedController()
	dc.QueueDamageAssignment([]engine.DamageAssignment{
		{Blocker: blocker1, Amount: 0},
		{Blocker: blocker2, Amount: 5},
	})

	var sink recordingSink
	g.SetSink(&sink)
	g.DealCombatDamage(dc)

	if g.Card(blocker1).Damage.Marked != 0 {
		t.Errorf("blocker1 damage = %d, want 0", g.Card(blocker1).Damage.Marked)
	}
	for _, e := range sink.events {
		if e.Kind == engine.DamageDealt && e.Target == engine.CardEntity(blocker1) {
			t.Errorf("a zero-amount assignment emitted a DamageDealt event: %+v", e)
		}
	}
}

// CR 702.19e: the same setup, but with trample -- once every blocker is
// gone, all the attacker's damage goes to the player instead.
func TestDealCombatDamageTrampleWithNoLiveBlockersHitsThePlayer(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	a, b := g.Players()[0], g.Players()[1]
	g.Player(a).Life, g.Player(b).Life = 20, 20
	g.SetTurnState(1, a, engine.Main1)
	// Toughness matches the attacker's power exactly, so the first-strike
	// step assigns it precisely lethal with no trample excess of its own --
	// isolating the regular step's own trample-with-nothing-left-to-block
	// case (CR 702.19e) from the ordinary single-blocker trample case
	// already covered above.
	attacker := g.NewCard(creatureDefPTKeywords(t, "5", "5", "Double Strike", "Trample"), a, engine.Battlefield)
	blocker := g.NewCard(creatureDefPT(t, "1", "5"), b, engine.Battlefield)
	declareOneAttackerAndBlocker(t, g, attacker, blocker)

	g.DealFirstStrikeDamage(engine.NewScriptedController())
	engine.CheckStateBasedActions(g, engine.NewScriptedController())

	if g.Card(blocker).Zone != engine.Graveyard {
		t.Fatalf("blocker zone = %v, want Graveyard (lethal first-strike damage)", g.Card(blocker).Zone)
	}
	if g.Player(b).Life != 20 {
		t.Fatalf("defender life after the first-strike step = %d, want unchanged at 20 (no trample excess yet)", g.Player(b).Life)
	}

	g.DealCombatDamage(engine.NewScriptedController())

	if g.Player(b).Life != 15 {
		t.Errorf("defender life = %d, want 15 (20 - 5, the regular step's trample with no blocker left at all)", g.Player(b).Life)
	}
}
