package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/carddb"
	"github.com/jczastkiewicz/crucible/internal/carddb/compile"
	"github.com/jczastkiewicz/crucible/internal/cardtype"
	"github.com/jczastkiewicz/crucible/internal/engine"
	"github.com/jczastkiewicz/crucible/internal/mana"
)

// damageDoneOnceWatcherDef builds an Enchantment carrying one Mode$
// DamageDoneOnce trigger naming extraParams, Execute$ chaining into a
// GainLife of 5 -- a fixed, distinctive amount so a test can tell "fired
// once for the whole batch" (25) apart from "fired once per source" (30
// for a two-source batch) without needing DamageAmount$'s own value at all.
func damageDoneOnceWatcherDef(t *testing.T, name, extraParams string) *compile.Card {
	t.Helper()

	raw := &carddb.Card{Filename: name}
	raw.Faces[0].Present = true
	raw.Faces[0].Name = name
	raw.Faces[0].Type = cardtype.Parse(attachmentTypeRegistry(t), "Enchantment")
	raw.Faces[0].Triggers = []string{
		"Mode$ DamageDoneOnce | " + extraParams + " | Execute$ TrigGain",
	}
	raw.Faces[0].SVars.Set("TrigGain", "DB$ GainLife | Defined$ You | LifeAmount$ 5")

	c, err := compile.Compile(raw)
	if err != nil {
		t.Fatalf("compile %q: %v", name, err)
	}
	return c
}

// TestDamageDoneOnceFiresOnceForDoubleBlockedAttacker proves CR 510.2's own
// "all combat damage is dealt simultaneously": an attacker blocked by two
// creatures takes damage from both in the same combat damage step, and
// Mode$ DamageDoneOnce fires once for the combined total (life +5), not
// once per blocker (which would be +10). ValidTarget$ Creature.YouCtrl
// isolates the attacker (the watcher's own controller's creature) from the
// two blockers (the opponent's), which are themselves separate targets in
// the identical table and would otherwise also fire the watcher.
func TestDamageDoneOnceFiresOnceForDoubleBlockedAttacker(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	a, b := g.Players()[0], g.Players()[1]
	g.SetTurnState(1, a, engine.Main1)
	g.Player(a).Life, g.Player(b).Life = 20, 20
	attacker := g.NewCard(creatureDefPT(t, "5", "5"), a, engine.Battlefield)
	blocker1 := g.NewCard(creatureDefPT(t, "1", "1"), b, engine.Battlefield)
	blocker2 := g.NewCard(creatureDefPT(t, "1", "1"), b, engine.Battlefield)
	g.NewCard(damageDoneOnceWatcherDef(t, "Test Watcher", "CombatDamage$ True | ValidTarget$ Creature.YouCtrl"), a, engine.Battlefield)

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
		{Blocker: blocker1, Amount: 2},
		{Blocker: blocker2, Amount: 3},
	})
	g.DealCombatDamage(dc)
	if err := g.ResolveStack(engine.NewRegistry(), dc); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}

	if g.Player(a).Life != 25 {
		t.Errorf("a's life = %d, want 25 -- Mode$ DamageDoneOnce must fire once for the attacker's combined 1+1 damage, not once per blocker", g.Player(a).Life)
	}
}

// TestDamageDoneOnceFiresOnceForMultipleUnblockedAttackersHittingOnePlayer
// proves the same aggregation on a player target: two unblocked attackers
// hitting the same defending player in one combat damage step fire the
// watcher once, not twice.
func TestDamageDoneOnceFiresOnceForMultipleUnblockedAttackersHittingOnePlayer(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	a, b := g.Players()[0], g.Players()[1]
	g.SetTurnState(1, a, engine.Main1)
	g.Player(a).Life, g.Player(b).Life = 20, 20
	attacker1 := g.NewCard(creatureDefPT(t, "3", "3"), a, engine.Battlefield)
	attacker2 := g.NewCard(creatureDefPT(t, "4", "4"), a, engine.Battlefield)
	g.NewCard(damageDoneOnceWatcherDef(t, "Test Watcher", "CombatDamage$ True | ValidTarget$ Player"), a, engine.Battlefield)

	ac := engine.NewScriptedController()
	ac.QueueAttackers([]engine.CardID{attacker1, attacker2})
	declareAttackers(t, g, ac)
	bc := engine.NewScriptedController()
	bc.QueueBlocks(nil)
	declareBlockers(t, g, bc)

	dc := engine.NewScriptedController()
	g.DealCombatDamage(dc)
	if err := g.ResolveStack(engine.NewRegistry(), dc); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}

	if g.Player(a).Life != 25 {
		t.Errorf("a's life = %d, want 25 -- Mode$ DamageDoneOnce must fire once for the defending player's combined 3+4 damage, not once per attacker", g.Player(a).Life)
	}
	if g.Player(b).Life != 13 {
		t.Errorf("b's life = %d, want 13 (20-3-4)", g.Player(b).Life)
	}
}

// TestDamageDoneOnceValidSourceFiltersTheSummedAmount proves
// damageDoneOnceAmount sums only entries whose own Source matches
// ValidSource$: a double-blocked attacker takes 1 damage from an Elf
// blocker and 1 from a Goblin blocker, but ValidSource$ Elf | DamageAmount$
// EQ1 only fires if the aggregation genuinely filtered by source before
// summing -- an unfiltered sum of 2 would fail EQ1 and never fire at all.
func TestDamageDoneOnceValidSourceFiltersTheSummedAmount(t *testing.T) {
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
	g.NewCard(damageDoneOnceWatcherDef(t, "Test Watcher", "CombatDamage$ True | ValidTarget$ Creature.YouCtrl | ValidSource$ Elf | DamageAmount$ EQ1"), a, engine.Battlefield)

	ac := engine.NewScriptedController()
	ac.QueueAttackers([]engine.CardID{attacker})
	declareAttackers(t, g, ac)
	bc := engine.NewScriptedController()
	bc.QueueBlocks([]engine.Block{
		{Blocker: elfBlocker, Attacker: attacker},
		{Blocker: goblinBlocker, Attacker: attacker},
	})
	declareBlockers(t, g, bc)

	dc := engine.NewScriptedController()
	dc.QueueDamageAssignment([]engine.DamageAssignment{
		{Blocker: elfBlocker, Amount: 2},
		{Blocker: goblinBlocker, Amount: 3},
	})
	g.DealCombatDamage(dc)
	if err := g.ResolveStack(engine.NewRegistry(), dc); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}

	if g.Player(a).Life != 25 {
		t.Errorf("a's life = %d, want 25 -- ValidSource$ Elf must filter the sum to the Elf blocker's own 1 damage before DamageAmount$ EQ1 is checked", g.Player(a).Life)
	}
}

// TestDamageDoneOnceFiresIndependentlyPerTargetForDealDamage proves the
// non-combat wiring (dealDamageEffect, dealdamageeffect.go): a DealDamage
// hitting every player (Defined$ Player) fires Mode$ DamageDoneOnce once
// per player -- grouped by target, not merged into one firing across
// different targets -- so a two-player game sees the watcher fire twice.
func TestDamageDoneOnceFiresIndependentlyPerTargetForDealDamage(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p, opp := g.Players()[0], g.Players()[1]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life, g.Player(opp).Life = 20, 20
	g.NewCard(damageDoneOnceWatcherDef(t, "Test Watcher", "ValidTarget$ Player"), p, engine.Battlefield)

	raw := &carddb.Card{Filename: "Test DealDamage Source"}
	raw.Faces[0].Present = true
	raw.Faces[0].Name = "Test DealDamage Source"
	raw.Faces[0].Type = cardtype.Parse(attachmentTypeRegistry(t), "Creature Elf")
	raw.Faces[0].Power, raw.Faces[0].Toughness = "1", "1"
	raw.Faces[0].ManaCost = mana.MustParse("R")
	raw.Faces[0].Triggers = []string{
		"Mode$ ChangesZone | Origin$ Any | Destination$ Battlefield | ValidCard$ Card.Self | Execute$ TrigDmg",
	}
	raw.Faces[0].SVars.Set("TrigDmg", "DB$ DealDamage | Defined$ Player | NumDmg$ 3")
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

	if g.Player(p).Life != 27 {
		t.Errorf("p's life = %d, want 27 (20 - 3 from DealDamage + 5 + 5 from the watcher firing once per player)", g.Player(p).Life)
	}
	if g.Player(opp).Life != 17 {
		t.Errorf("opp's life = %d, want 17 (20 - 3 from DealDamage)", g.Player(opp).Life)
	}
}

// TestDamageDoneOnceSkipsCombatDamageMismatch proves CombatDamage$ gates
// the trigger: a script-driven (non-combat) DealDamage must not fire a
// watcher naming CombatDamage$ True.
func TestDamageDoneOnceSkipsCombatDamageMismatch(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p, opp := g.Players()[0], g.Players()[1]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life, g.Player(opp).Life = 20, 20
	g.NewCard(damageDoneOnceWatcherDef(t, "Test Watcher", "CombatDamage$ True | ValidTarget$ Player"), p, engine.Battlefield)

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

	if g.Player(p).Life != 20 {
		t.Errorf("p's life = %d, want unchanged 20 -- CombatDamage$ True must not match a non-combat DealDamage", g.Player(p).Life)
	}
}

// TestDamageDoneOnceSkipsLineNamingResolvedLimit proves a real, unresolved
// param (ResolvedLimit$, 2 of the corpus's own 206 real lines) is skipped
// rather than firing unconditionally (PORT-8/GO-7).
func TestDamageDoneOnceSkipsLineNamingResolvedLimit(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	a, b := g.Players()[0], g.Players()[1]
	g.SetTurnState(1, a, engine.Main1)
	g.Player(a).Life, g.Player(b).Life = 20, 20
	attacker1 := g.NewCard(creatureDefPT(t, "3", "3"), a, engine.Battlefield)
	attacker2 := g.NewCard(creatureDefPT(t, "4", "4"), a, engine.Battlefield)
	g.NewCard(damageDoneOnceWatcherDef(t, "Test Watcher", "CombatDamage$ True | ValidTarget$ Player | ResolvedLimit$ 1"), a, engine.Battlefield)

	ac := engine.NewScriptedController()
	ac.QueueAttackers([]engine.CardID{attacker1, attacker2})
	declareAttackers(t, g, ac)
	bc := engine.NewScriptedController()
	bc.QueueBlocks(nil)
	declareBlockers(t, g, bc)

	dc := engine.NewScriptedController()
	g.DealCombatDamage(dc)
	if err := g.ResolveStack(engine.NewRegistry(), dc); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}

	if g.Player(a).Life != 20 {
		t.Errorf("a's life = %d, want unchanged 20 -- ResolvedLimit$ must skip the whole line rather than firing unconditionally", g.Player(a).Life)
	}
}
