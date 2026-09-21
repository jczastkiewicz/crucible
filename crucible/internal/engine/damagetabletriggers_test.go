package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/carddb"
	"github.com/jczastkiewicz/crucible/internal/carddb/compile"
	"github.com/jczastkiewicz/crucible/internal/cardtype"
	"github.com/jczastkiewicz/crucible/internal/engine"
	"github.com/jczastkiewicz/crucible/internal/mana"
)

// damageTableWatcherDef builds an Enchantment carrying one trigger of mode
// naming extraParams, Execute$ chaining into a GainLife of 5 -- the
// identical distinctive-amount shape damageDoneOnceWatcherDef
// (damagedoneonce_test.go) already established, reused for
// DamageDealtOnce's and DamageAll's own tests here.
func damageTableWatcherDef(t *testing.T, name, mode, extraParams string) *compile.Card {
	t.Helper()

	raw := &carddb.Card{Filename: name}
	raw.Faces[0].Present = true
	raw.Faces[0].Name = name
	raw.Faces[0].Type = cardtype.Parse(attachmentTypeRegistry(t), "Enchantment")
	raw.Faces[0].Triggers = []string{
		"Mode$ " + mode + " | " + extraParams + " | Execute$ TrigGain",
	}
	raw.Faces[0].SVars.Set("TrigGain", "DB$ GainLife | Defined$ You | LifeAmount$ 5")

	c, err := compile.Compile(raw)
	if err != nil {
		t.Fatalf("compile %q: %v", name, err)
	}
	return c
}

// TestDamageDealtOnceFiresOnceForSourceHittingMultipleTargets proves
// Mode$ DamageDoneOnce's own source-grouped sibling: a gang-blocked
// attacker splitting its power between two blockers is one source dealing
// damage to two targets in the same combat damage step, and
// Mode$ DamageDealtOnce fires once for the combined total (life +5), not
// once per target hit (which would be +10). ValidSource$ Creature.YouCtrl
// isolates the attacker (the watcher's own controller's creature) from the
// two blockers, which are themselves separate sources in the identical
// table and would otherwise also fire the watcher for their own single hit
// on the attacker.
func TestDamageDealtOnceFiresOnceForSourceHittingMultipleTargets(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	a, b := g.Players()[0], g.Players()[1]
	g.SetTurnState(1, a, engine.Main1)
	g.Player(a).Life, g.Player(b).Life = 20, 20
	attacker := g.NewCard(creatureDefPT(t, "5", "5"), a, engine.Battlefield)
	blocker1 := g.NewCard(creatureDefPT(t, "1", "1"), b, engine.Battlefield)
	blocker2 := g.NewCard(creatureDefPT(t, "1", "1"), b, engine.Battlefield)
	g.NewCard(damageTableWatcherDef(t, "Test Watcher", "DamageDealtOnce", "CombatDamage$ True | ValidSource$ Creature.YouCtrl"), a, engine.Battlefield)

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
		{Blocker: blocker1, Amount: 2},
		{Blocker: blocker2, Amount: 3},
	})
	g.DealCombatDamage(dc)
	if err := g.ResolveStack(engine.NewRegistry(), dc); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}

	if g.Player(a).Life != 25 {
		t.Errorf("a's life = %d, want 25 -- Mode$ DamageDealtOnce must fire once for the attacker's combined 2+3 damage, not once per blocker hit", g.Player(a).Life)
	}
}

// TestDamageDealtOnceValidTargetFiltersTheSummedAmount proves
// damageDealtOnceAmount genuinely filters by ValidTarget$ before summing,
// and gates the whole line on that filtered sum being positive: the
// attacker deals 0 damage to an Elf blocker and 5 to a Goblin blocker (a
// legal AssignCombatDamage answer), and ValidTarget$ Creature.Elf must
// therefore see a filtered sum of 0 and not fire -- an unfiltered
// implementation (summing every target the source hit) would see 5 and
// fire regardless of ValidTarget$'s own restriction.
func TestDamageDealtOnceValidTargetFiltersTheSummedAmount(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	a, b := g.Players()[0], g.Players()[1]
	g.SetTurnState(1, a, engine.Main1)
	g.Player(a).Life, g.Player(b).Life = 20, 20
	attacker := g.NewCard(creatureDefPT(t, "5", "5"), a, engine.Battlefield)
	elfBlocker := g.NewCard(creatureDefPT(t, "1", "1"), b, engine.Battlefield)
	goblinDef := &compile.Card{Name: "Test Goblin Blocker"}
	goblinDef.Faces[0].Type = cardtype.Parse(attachmentTypeRegistry(t), "Creature Goblin")
	goblinDef.Faces[0].Power, goblinDef.Faces[0].Toughness = "1", "1"
	goblinBlocker := g.NewCard(goblinDef, b, engine.Battlefield)
	g.NewCard(damageTableWatcherDef(t, "Test Watcher", "DamageDealtOnce", "CombatDamage$ True | ValidSource$ Creature.YouCtrl | ValidTarget$ Creature.Elf"), a, engine.Battlefield)

	ac := engine.NewScriptedController()
	ac.QueueAttackers([]engine.CardID{attacker})
	g.DeclareCombatAttackers(ac)
	bc := engine.NewScriptedController()
	bc.QueueBlocks([]engine.Block{
		{Blocker: elfBlocker, Attacker: attacker},
		{Blocker: goblinBlocker, Attacker: attacker},
	})
	g.DeclareCombatBlockers(bc)

	dc := engine.NewScriptedController()
	dc.QueueDamageAssignment([]engine.DamageAssignment{
		{Blocker: elfBlocker, Amount: 0},
		{Blocker: goblinBlocker, Amount: 5},
	})
	g.DealCombatDamage(dc)
	if err := g.ResolveStack(engine.NewRegistry(), dc); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}

	if g.Player(a).Life != 20 {
		t.Errorf("a's life = %d, want unchanged 20 -- ValidTarget$ Creature.Elf must filter the sum to the Elf blocker's own 0 damage, not the Goblin blocker's 5", g.Player(a).Life)
	}
}

// TestDamageDealtOnceSkipsLineNamingActivationLimit proves a real,
// unresolved param (ActivationLimit$, 1 of the corpus's own 49 real lines)
// is skipped rather than firing unconditionally (PORT-8/GO-7).
func TestDamageDealtOnceSkipsLineNamingActivationLimit(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	a, b := g.Players()[0], g.Players()[1]
	g.SetTurnState(1, a, engine.Main1)
	g.Player(a).Life, g.Player(b).Life = 20, 20
	attacker := g.NewCard(creatureDefPT(t, "3", "3"), a, engine.Battlefield)
	g.NewCard(damageTableWatcherDef(t, "Test Watcher", "DamageDealtOnce", "CombatDamage$ True | ValidSource$ Creature.YouCtrl | ActivationLimit$ 1"), a, engine.Battlefield)

	ac := engine.NewScriptedController()
	ac.QueueAttackers([]engine.CardID{attacker})
	g.DeclareCombatAttackers(ac)
	bc := engine.NewScriptedController()
	bc.QueueBlocks(nil)
	g.DeclareCombatBlockers(bc)

	dc := engine.NewScriptedController()
	g.DealCombatDamage(dc)
	if err := g.ResolveStack(engine.NewRegistry(), dc); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}

	if g.Player(a).Life != 20 {
		t.Errorf("a's life = %d, want unchanged 20 -- ActivationLimit$ must skip the whole line rather than firing unconditionally", g.Player(a).Life)
	}
}

// TestDamageAllFiresOnceRegardlessOfGrouping proves Mode$ DamageAll fires
// once for the whole damage-dealing action with no grouping at all: a
// double-blocked attacker produces four separate table entries (attacker to
// each blocker, each blocker back to the attacker), and a watcher naming
// neither ValidSource$ nor ValidTarget$ still fires exactly once, not once
// per entry.
func TestDamageAllFiresOnceRegardlessOfGrouping(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	a, b := g.Players()[0], g.Players()[1]
	g.SetTurnState(1, a, engine.Main1)
	g.Player(a).Life, g.Player(b).Life = 20, 20
	attacker := g.NewCard(creatureDefPT(t, "5", "5"), a, engine.Battlefield)
	blocker1 := g.NewCard(creatureDefPT(t, "1", "1"), b, engine.Battlefield)
	blocker2 := g.NewCard(creatureDefPT(t, "1", "1"), b, engine.Battlefield)
	g.NewCard(damageTableWatcherDef(t, "Test Watcher", "DamageAll", "CombatDamage$ True"), a, engine.Battlefield)

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
		{Blocker: blocker1, Amount: 2},
		{Blocker: blocker2, Amount: 3},
	})
	g.DealCombatDamage(dc)
	if err := g.ResolveStack(engine.NewRegistry(), dc); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}

	if g.Player(a).Life != 25 {
		t.Errorf("a's life = %d, want 25 -- Mode$ DamageAll must fire once for the whole action's four exchanges, not once per exchange", g.Player(a).Life)
	}
}

// TestDamageAllRespectsValidSourceAndValidTargetFilter proves ValidSource$/
// ValidTarget$ actually gate Mode$ DamageAll rather than it firing on any
// damage at all: a watcher naming ValidTarget$ Player never fires against
// an all-creature double block (no player was ever a target in the table),
// while an identical watcher naming ValidSource$ Creature.YouCtrl does,
// since the attacker (the watcher's own controller's creature) really did
// deal damage in the action.
func TestDamageAllRespectsValidSourceAndValidTargetFilter(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	a, b := g.Players()[0], g.Players()[1]
	g.SetTurnState(1, a, engine.Main1)
	g.Player(a).Life, g.Player(b).Life = 20, 20
	attacker := g.NewCard(creatureDefPT(t, "3", "3"), a, engine.Battlefield)
	blocker := g.NewCard(creatureDefPT(t, "1", "1"), b, engine.Battlefield)
	g.NewCard(damageTableWatcherDef(t, "Test No-Match Watcher", "DamageAll", "CombatDamage$ True | ValidTarget$ Player"), a, engine.Battlefield)

	ac := engine.NewScriptedController()
	ac.QueueAttackers([]engine.CardID{attacker})
	g.DeclareCombatAttackers(ac)
	bc := engine.NewScriptedController()
	bc.QueueBlocks([]engine.Block{{Blocker: blocker, Attacker: attacker}})
	g.DeclareCombatBlockers(bc)

	dc := engine.NewScriptedController()
	g.DealCombatDamage(dc)
	if err := g.ResolveStack(engine.NewRegistry(), dc); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}

	if g.Player(a).Life != 20 {
		t.Errorf("a's life = %d, want unchanged 20 -- ValidTarget$ Player must not match an all-creature combat", g.Player(a).Life)
	}
}

// TestDamageAllFiresForDealDamageScriptEffect proves the non-combat wiring
// (dealDamageEffect, dealdamageeffect.go): a DealDamage hitting a single
// player still fires Mode$ DamageAll once through checkDamageTableTriggers,
// the identical shared caller Mode$ DamageDoneOnce/DamageDealtOnce already
// use.
func TestDamageAllFiresForDealDamageScriptEffect(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p, opp := g.Players()[0], g.Players()[1]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life, g.Player(opp).Life = 20, 20
	g.NewCard(damageTableWatcherDef(t, "Test Watcher", "DamageAll", "ValidTarget$ Opponent"), p, engine.Battlefield)

	raw := &carddb.Card{Filename: "Test DealDamage Source"}
	raw.Faces[0].Present = true
	raw.Faces[0].Name = "Test DealDamage Source"
	raw.Faces[0].Type = cardtype.Parse(attachmentTypeRegistry(t), "Creature Elf")
	raw.Faces[0].Power, raw.Faces[0].Toughness = "1", "1"
	raw.Faces[0].ManaCost = mana.MustParse("R")
	raw.Faces[0].Triggers = []string{
		"Mode$ ChangesZone | Origin$ Any | Destination$ Battlefield | ValidCard$ Card.Self | Execute$ TrigDmg",
	}
	raw.Faces[0].SVars.Set("TrigDmg", "DB$ DealDamage | Defined$ Opponent | NumDmg$ 3")
	def, err := compile.Compile(raw)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}

	c := engine.NewScriptedController()
	g.Player(p).ManaPool.Add(mana.Red, 1)
	source := g.NewCard(def, p, engine.Hand)
	if !g.CastSpell(p, source, c) {
		t.Fatal("CastSpell failed casting a creature with exactly enough mana")
	}
	if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}

	if g.Player(p).Life != 25 {
		t.Errorf("p's life = %d, want 25 -- Mode$ DamageAll must fire for a non-combat DealDamage too", g.Player(p).Life)
	}
	if g.Player(opp).Life != 17 {
		t.Errorf("opp's life = %d, want 17 (20-3)", g.Player(opp).Life)
	}
}
