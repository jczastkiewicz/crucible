package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
)

// CR 702.15b: damage dealt by a source with lifelink also makes its controller
// gain that much life; here an unblocked lifelink attacker hits the player.
func TestLifelinkCombatDamageGainsLifeForTheController(t *testing.T) {
	t.Parallel()

	g, a, b := combatGame(t)
	attacker := g.NewCard(creatureDefPTKeywords(t, "3", "3", "Lifelink"), a, engine.Battlefield)
	declareAttacking(t, g, attacker)
	bc := engine.NewScriptedController()
	bc.QueueBlocks(nil)
	declareBlockers(t, g, bc)
	g.DealCombatDamage(engine.NewScriptedController())

	if got := g.Player(b).Life; got != 17 {
		t.Errorf("defender life = %d, want 17", got)
	}
	if got := g.Player(a).Life; got != 23 {
		t.Errorf("attacker's controller life = %d, want 23", got)
	}
}

// Lifelink counts the damage actually dealt to a creature too (CR 702.15b,
// 120.3): a 3-power lifelinker blocked by a 1/1 deals 3 and gains 3 even
// though only 1 was lethal.
func TestLifelinkGainsForDamageDealtToCreatures(t *testing.T) {
	t.Parallel()

	g, a, _ := combatGame(t)
	attacker := g.NewCard(creatureDefPTKeywords(t, "3", "3", "Lifelink"), a, engine.Battlefield)
	blocker := g.NewCard(creatureDefPT(t, "1", "1"), g.Players()[1], engine.Battlefield)
	declareAttacking(t, g, attacker)
	bc := engine.NewScriptedController()
	bc.QueueBlocks([]engine.Block{{Blocker: blocker, Attacker: attacker}})
	declareBlockers(t, g, bc)
	g.DealCombatDamage(engine.NewScriptedController())

	if got := g.Player(a).Life; got != 23 {
		t.Errorf("life = %d, want 23 -- lifelink gains the full 3 damage dealt", got)
	}
}

// CR 702.15e: a source's damage in one event is one life gain. A lifelinker
// split across two blockers triggers "whenever you gain life" once, not once
// per blocker (GameAction.java:2734 sums per source).
func TestLifelinkIsOneGainPerSourceAcrossBlockers(t *testing.T) {
	t.Parallel()

	g, a, b := combatGame(t)
	attacker := g.NewCard(creatureDefPTKeywords(t, "3", "3", "Lifelink"), a, engine.Battlefield)
	g.NewCard(scriptDef(t, "Test Watcher", "Creature Elf",
		"T:Mode$ LifeGained | ValidPlayer$ You | TriggerZones$ Battlefield | Execute$ TrigDraw",
		"SVar:TrigDraw:DB$ Draw | Defined$ You | NumCards$ 1"), a, engine.Battlefield)
	for range 3 {
		g.NewCard(creatureDefPT(t, "1", "1"), a, engine.Library)
	}
	blocker1 := g.NewCard(creatureDefPT(t, "1", "1"), b, engine.Battlefield)
	blocker2 := g.NewCard(creatureDefPT(t, "1", "1"), b, engine.Battlefield)
	declareAttacking(t, g, attacker)
	bc := engine.NewScriptedController()
	bc.QueueBlocks([]engine.Block{{Blocker: blocker1, Attacker: attacker}, {Blocker: blocker2, Attacker: attacker}})
	declareBlockers(t, g, bc)

	dc := engine.NewScriptedController()
	dc.QueueDamageAssignment([]engine.DamageAssignment{{Blocker: blocker1, Amount: 1}, {Blocker: blocker2, Amount: 2}})
	g.DealCombatDamage(dc)
	if err := g.ResolveStack(engine.NewRegistry(), dc); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}

	if got := g.Player(a).Life; got != 23 {
		t.Errorf("life = %d, want 23", got)
	}
	if got := len(g.Zone(engine.Hand, a).Cards()); got != 1 {
		t.Errorf("cards in hand = %d, want 1 -- one LifeGained trigger for one gain", got)
	}
}

// Without the keyword no life is gained.
func TestLifelinkGainsNothingWithoutTheKeyword(t *testing.T) {
	t.Parallel()

	g, a, b := combatGame(t)
	plain := g.NewCard(creatureDefPT(t, "3", "3"), a, engine.Battlefield)
	declareAttacking(t, g, plain)
	bc := engine.NewScriptedController()
	bc.QueueBlocks(nil)
	declareBlockers(t, g, bc)
	g.DealCombatDamage(engine.NewScriptedController())

	if g.Player(a).Life != 20 || g.Player(b).Life != 17 {
		t.Errorf("life a=%d b=%d, want 20 and 17", g.Player(a).Life, g.Player(b).Life)
	}
}

// CR 702.15b covers every damage source, not only combat: a lifelink
// creature's own DealDamage ability gains its controller the damage it deals.
func TestLifelinkNoncombatDamageGainsLife(t *testing.T) {
	t.Parallel()

	g, a, b := combatGame(t)
	def := scriptDef(t, "Test Pinger", "Creature Elf",
		"K:Lifelink",
		"A:AB$ DealDamage | Cost$ T | ValidTgts$ Player | NumDmg$ 2")
	pinger := g.NewCard(def, a, engine.Battlefield)
	g.Card(pinger).SummonSick = false

	c := engine.NewScriptedController()
	c.QueueTargets([]engine.EntityID{engine.PlayerEntity(b)})
	if !g.ActivateAbility(a, pinger, 0, c) {
		t.Fatal("ActivateAbility failed")
	}
	if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}

	if g.Player(b).Life != 18 || g.Player(a).Life != 22 {
		t.Errorf("life a=%d b=%d, want 22 and 18", g.Player(a).Life, g.Player(b).Life)
	}
}
