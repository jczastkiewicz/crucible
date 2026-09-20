package engine_test

import (
	"strings"
	"testing"

	"github.com/jczastkiewicz/crucible/internal/carddb"
	"github.com/jczastkiewicz/crucible/internal/carddb/compile"
	"github.com/jczastkiewicz/crucible/internal/cardtype"
	"github.com/jczastkiewicz/crucible/internal/engine"
	"github.com/jczastkiewicz/crucible/internal/mana"
)

// replacementHostDef builds a *compile.Card carrying one real R:Event$
// Moved line and the SVar its own ReplaceWith$ names, compiled through the
// real pipeline the same reason continuousDef (continuous_test.go) is.
// checkMovedReplacement is unexported, so every case here is driven through
// Game.PlayLand, its only caller besides castspell.go's own permanentEffect/
// attachEffect (TEST-1).
func replacementHostDef(t *testing.T, name, cardType, replacement, svarName, svarBody string) *compile.Card {
	t.Helper()

	reg, err := cardtype.LoadRegistry(strings.NewReader("[CreatureTypes]\nElf\n"))
	if err != nil {
		t.Fatalf("LoadRegistry: %v", err)
	}
	raw := &carddb.Card{Filename: name}
	raw.Faces[0].Present = true
	raw.Faces[0].Name = name
	raw.Faces[0].Type = cardtype.Parse(reg, cardType)
	raw.Faces[0].Replacements = []string{replacement}
	raw.Faces[0].SVars.Set(svarName, svarBody)
	// A SubAbility$ chain (TestCheckMovedReplacementSkipsSubAbilityChain)
	// still has to compile even though this file never resolves it -- M3's
	// own "a reference the compiler never follows is one the corpus gate
	// can never find dangling" rule -- so every case here defines it.
	raw.Faces[0].SVars.Set("DBAddCounter", "DB$ Cleanup")

	c, err := compile.Compile(raw)
	if err != nil {
		t.Fatalf("compile %q: %v", name, err)
	}
	return c
}

// creatureReplacementDef is replacementHostDef's own castable counterpart:
// a Creature with a mana cost, so CastSpell/ResolveStack drives it through
// permanentEffect.Resolve (castspell.go) rather than Game.PlayLand --
// checkMovedReplacement's OTHER real call site.
func creatureReplacementDef(t *testing.T, name, cost, replacement, svarName, svarBody string) *compile.Card {
	t.Helper()
	def := replacementHostDef(t, name, "Creature Elf", replacement, svarName, svarBody)
	def.Faces[0].ManaCost = mana.MustParse(cost)
	def.Faces[0].Power, def.Faces[0].Toughness = "2", "2"
	return def
}

// replacementCreatureDefPT builds a Creature *compile.Card carrying zero or
// more real R: lines and no SVar -- untapReplacementMatches/
// damagePreventionMatches resolve a Layer$ CantHappen/Prevent$ True line
// directly, unlike checkMovedReplacement's own ETBTapped shape
// (replacementHostDef, above), which always names a ReplaceWith$ SVar.
func replacementCreatureDefPT(t *testing.T, name, power, toughness string, replacements ...string) *compile.Card {
	t.Helper()

	reg, err := cardtype.LoadRegistry(strings.NewReader("[CreatureTypes]\nElf\n"))
	if err != nil {
		t.Fatalf("LoadRegistry: %v", err)
	}
	raw := &carddb.Card{Filename: name}
	raw.Faces[0].Present = true
	raw.Faces[0].Name = name
	raw.Faces[0].Type = cardtype.Parse(reg, "Creature Elf")
	raw.Faces[0].Power, raw.Faces[0].Toughness = power, toughness
	raw.Faces[0].Replacements = replacements

	c, err := compile.Compile(raw)
	if err != nil {
		t.Fatalf("compile %q: %v", name, err)
	}
	return c
}

// replacementEnchantmentDef is replacementCreatureDefPT's own noncreature
// twin, for a watcher that carries the replacement without itself being the
// affected permanent -- Test Untap Lock/Test Damage Shield below, the
// identical role Test Tap Enforcer already has in
// TestCheckMovedReplacementAppliesToOtherPermanentsEntering.
func replacementEnchantmentDef(t *testing.T, name string, replacements ...string) *compile.Card {
	t.Helper()

	reg, err := cardtype.LoadRegistry(strings.NewReader("[CreatureTypes]\nElf\n"))
	if err != nil {
		t.Fatalf("LoadRegistry: %v", err)
	}
	raw := &carddb.Card{Filename: name}
	raw.Faces[0].Present = true
	raw.Faces[0].Name = name
	raw.Faces[0].Type = cardtype.Parse(reg, "Enchantment")
	raw.Faces[0].Replacements = replacements

	c, err := compile.Compile(raw)
	if err != nil {
		t.Fatalf("compile %q: %v", name, err)
	}
	return c
}

// TestCastSpellCreatureEntersTappedViaReplacement proves checkMovedReplacement
// is wired at castspell.go's own permanentEffect.Resolve too, not just
// Game.PlayLand -- a creature carrying the identical ETBTapped shape a real
// corpus land does enters the battlefield already tapped, through the stack.
func TestCastSpellCreatureEntersTappedViaReplacement(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a")
	p := g.Players()[0]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).ManaPool.Add(mana.Red, 1)
	creature := g.NewCard(creatureReplacementDef(t, "Test Tapped Creature", "R",
		"Event$ Moved | ValidCard$ Card.Self | Destination$ Battlefield | ReplaceWith$ ETBTapped",
		"ETBTapped", "DB$ Tap | Defined$ Self | ETB$ True"), p, engine.Hand)
	c := engine.NewScriptedController()

	if !g.CastSpell(p, creature, c) {
		t.Fatal("CastSpell failed casting a creature with exactly enough mana")
	}
	if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if g.Card(creature).Zone != engine.Battlefield {
		t.Fatalf("card zone after resolving = %v, want Battlefield", g.Card(creature).Zone)
	}
	if !g.Card(creature).Tapped {
		t.Error("Tapped = false, want true -- ReplaceWith$ ETBTapped must tap the creature as it enters through the stack")
	}
}

// TestPlayLandEntersTappedViaReplacement proves the corpus's own dominant
// real shape (587 of 618 real lines this port resolves): a land's own
// R:Event$ Moved | ValidCard$ Card.Self | Destination$ Battlefield |
// ReplaceWith$ ETBTapped, naming an SVar whose body is a bare DB$ Tap, taps
// the land as it enters -- before checkETBTriggers ever sees it.
func TestPlayLandEntersTappedViaReplacement(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a")
	p := g.Players()[0]
	g.SetTurnState(1, p, engine.Main1)
	land := g.NewCard(replacementHostDef(t, "Test Tapland", "Land",
		"Event$ Moved | ValidCard$ Card.Self | Destination$ Battlefield | ReplaceWith$ ETBTapped",
		"ETBTapped", "DB$ Tap | Defined$ Self | ETB$ True"), p, engine.Hand)

	if !g.PlayLand(p, land, engine.NewScriptedController()) {
		t.Fatal("PlayLand failed")
	}
	if !g.Card(land).Tapped {
		t.Error("Tapped = false, want true -- ReplaceWith$ ETBTapped must tap the land as it enters")
	}
}

// TestPlayLandDoesNotEnterTappedWhenDestinationDoesNotMatch proves
// Destination$ is checked, not assumed: a line naming Destination$
// Graveyard must not fire on a move to Battlefield.
func TestPlayLandDoesNotEnterTappedWhenDestinationDoesNotMatch(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a")
	p := g.Players()[0]
	g.SetTurnState(1, p, engine.Main1)
	land := g.NewCard(replacementHostDef(t, "Test Other Land", "Land",
		"Event$ Moved | ValidCard$ Card.Self | Destination$ Graveyard | ReplaceWith$ ETBTapped",
		"ETBTapped", "DB$ Tap | Defined$ Self | ETB$ True"), p, engine.Hand)

	g.PlayLand(p, land, engine.NewScriptedController())

	if g.Card(land).Tapped {
		t.Error("Tapped = true, want false -- Destination$ Graveyard must not match a move to Battlefield")
	}
}

// TestCheckMovedReplacementSkipsSubAbilityChain proves a ReplaceWith$ SVar
// carrying anything past a bare DB$ Tap/Defined$/ETB$ (SubAbility$, 5 of the
// corpus's 624 real ETBTapped lines -- a chained counter grant) skips the
// whole line rather than tapping unconditionally and dropping the rest of
// what the line says (PORT-8/GO-7).
func TestCheckMovedReplacementSkipsSubAbilityChain(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a")
	p := g.Players()[0]
	g.SetTurnState(1, p, engine.Main1)
	land := g.NewCard(replacementHostDef(t, "Test Chained Tapland", "Land",
		"Event$ Moved | ValidCard$ Card.Self | Destination$ Battlefield | ReplaceWith$ ETBTapped",
		"ETBTapped", "DB$ Tap | Defined$ Self | ETB$ True | SubAbility$ DBAddCounter"), p, engine.Hand)

	g.PlayLand(p, land, engine.NewScriptedController())

	if g.Card(land).Tapped {
		t.Error("Tapped = true, want false -- a chained SubAbility$ is not resolvable, so the whole line must be skipped")
	}
}

// TestCheckMovedReplacementTapsCheckland proves LandTapped's own real shape
// (140 real DB$ Tap lines, "enters tapped unless you control a Mountain or a
// Forest," Rootbound Crag's own text) resolves now:
// tapAbilityResolvesTap/subAbilityConditionMet (replacement.go, condition.go)
// evaluate ConditionPresent$ Mountain.YouCtrl | ConditionCompare$ EQ0 against
// the actual battlefield -- p controls no Mountain, so the count is 0, EQ0
// holds, and the checkland enters tapped.
func TestCheckMovedReplacementTapsCheckland(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a")
	p := g.Players()[0]
	g.SetTurnState(1, p, engine.Main1)
	land := g.NewCard(replacementHostDef(t, "Test Checkland", "Land",
		"Event$ Moved | ValidCard$ Card.Self | Destination$ Battlefield | ReplaceWith$ LandTapped",
		"LandTapped", "DB$ Tap | Defined$ Self | ETB$ True | ConditionPresent$ Mountain.YouCtrl | ConditionCompare$ EQ0"), p, engine.Hand)

	g.PlayLand(p, land, engine.NewScriptedController())

	if !g.Card(land).Tapped {
		t.Error("Tapped = false, want true -- p controls no Mountain, so ConditionPresent$ Mountain.YouCtrl | ConditionCompare$ EQ0 holds")
	}
}

// TestCheckMovedReplacementDoesNotTapChecklandWhenConditionUnmet proves the
// negative control: with a Mountain already on the battlefield, the count is
// 1, EQ0 fails, and the checkland enters untapped -- the same real card,
// genuinely evaluated both ways rather than one fixed outcome.
func TestCheckMovedReplacementDoesNotTapChecklandWhenConditionUnmet(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a")
	p := g.Players()[0]
	g.SetTurnState(1, p, engine.Main1)
	g.NewCard(landDef(t, "Mountain", "Basic Land Mountain"), p, engine.Battlefield)
	land := g.NewCard(replacementHostDef(t, "Test Checkland", "Land",
		"Event$ Moved | ValidCard$ Card.Self | Destination$ Battlefield | ReplaceWith$ LandTapped",
		"LandTapped", "DB$ Tap | Defined$ Self | ETB$ True | ConditionPresent$ Mountain.YouCtrl | ConditionCompare$ EQ0"), p, engine.Hand)

	g.PlayLand(p, land, engine.NewScriptedController())

	if g.Card(land).Tapped {
		t.Error("Tapped = true, want false -- p already controls a Mountain, so ConditionPresent$ Mountain.YouCtrl | ConditionCompare$ EQ0 fails")
	}
}

// TestCheckMovedReplacementTapsWhenConditionCheckSVarIsMet proves the other
// resolvable Condition-family shape: ConditionCheckSVar$/ConditionSVarCompare$,
// resolveNamedAmount's own literal-SVar path (amount.go), the identical
// mechanism CardTraitBase's own CheckSVar$/SVarCompare$ already uses.
func TestCheckMovedReplacementTapsWhenConditionCheckSVarIsMet(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a")
	p := g.Players()[0]
	g.SetTurnState(1, p, engine.Main1)
	land := g.NewCard(checklandCheckSVarDef(t, "1"), p, engine.Hand)

	g.PlayLand(p, land, engine.NewScriptedController())

	if !g.Card(land).Tapped {
		t.Error("Tapped = false, want true -- ConditionCheckSVar$ X | ConditionSVarCompare$ GE1 holds (X is 1)")
	}
}

// TestCheckMovedReplacementDoesNotTapWhenConditionCheckSVarIsNotMet proves
// the negative control for the identical shape.
func TestCheckMovedReplacementDoesNotTapWhenConditionCheckSVarIsNotMet(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a")
	p := g.Players()[0]
	g.SetTurnState(1, p, engine.Main1)
	land := g.NewCard(checklandCheckSVarDef(t, "0"), p, engine.Hand)

	g.PlayLand(p, land, engine.NewScriptedController())

	if g.Card(land).Tapped {
		t.Error("Tapped = true, want false -- ConditionCheckSVar$ X | ConditionSVarCompare$ GE1 fails (X is 0)")
	}
}

// checklandCheckSVarDef builds a *compile.Card for a land whose own
// LandTapped SVar carries ConditionCheckSVar$ X | ConditionSVarCompare$ GE1,
// X a literal integer set before compiling (compile.Face.Amounts is parsed
// once at compile time, so a *compile.Card's own SVars cannot be edited
// afterward the way replacementHostDef's own single-SVar shape might
// suggest).
func checklandCheckSVarDef(t *testing.T, xValue string) *compile.Card {
	t.Helper()

	reg, err := cardtype.LoadRegistry(strings.NewReader("[CreatureTypes]\nElf\n"))
	if err != nil {
		t.Fatalf("LoadRegistry: %v", err)
	}
	raw := &carddb.Card{Filename: "Test SVar Checkland"}
	raw.Faces[0].Present = true
	raw.Faces[0].Name = "Test SVar Checkland"
	raw.Faces[0].Type = cardtype.Parse(reg, "Land")
	raw.Faces[0].Replacements = []string{
		"Event$ Moved | ValidCard$ Card.Self | Destination$ Battlefield | ReplaceWith$ LandTapped",
	}
	raw.Faces[0].SVars.Set("LandTapped", "DB$ Tap | Defined$ Self | ETB$ True | ConditionCheckSVar$ X | ConditionSVarCompare$ GE1")
	raw.Faces[0].SVars.Set("X", xValue)

	c, err := compile.Compile(raw)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	return c
}

// TestCheckMovedReplacementAppliesToOtherPermanentsEntering proves the
// "other" half (31 of the corpus's 624 real ETBTapped lines): a permanent
// already on the battlefield can make an OPPONENT's land enter tapped, the
// identical own/other split checkETBTriggers/otherETBTriggerMatches already
// established for triggers.
func TestCheckMovedReplacementAppliesToOtherPermanentsEntering(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	a, b := g.Players()[0], g.Players()[1]
	g.SetTurnState(1, b, engine.Main1)
	g.NewCard(replacementHostDef(t, "Test Tap Enforcer", "Enchantment",
		"Event$ Moved | ValidCard$ Land.OppCtrl | Destination$ Battlefield | ReplaceWith$ ETBTapped",
		"ETBTapped", "DB$ Tap | Defined$ ReplacedCard | ETB$ True"), a, engine.Battlefield)
	land := g.NewCard(replacementHostDef(t, "Test Opponent Land", "Land",
		"Event$ Moved | ValidCard$ Card.NonLegendary | Destination$ Graveyard | ReplaceWith$ ETBTapped",
		"ETBTapped", "DB$ Tap | Defined$ Self | ETB$ True"), b, engine.Hand)

	if !g.PlayLand(b, land, engine.NewScriptedController()) {
		t.Fatal("PlayLand failed")
	}
	if !g.Card(land).Tapped {
		t.Error("Tapped = false, want true -- Test Tap Enforcer's own ValidCard$ Land.OppCtrl must reach a's opponent's land")
	}
}

// TestUntapBlockedBySelfCantHappenReplacement proves untapBlocked's own
// simplest real shape (CR 502.3/614.17): a permanent naming
// Event$ Untap | Layer$ CantHappen against itself stays tapped through its
// own controller's untap step, while summoning sickness still clears --
// untapStep's own doc comment on why the two are independent.
func TestUntapBlockedBySelfCantHappenReplacement(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a")
	p := g.Players()[0]
	c := g.NewCard(replacementCreatureDefPT(t, "Test Locked Creature", "2", "2",
		"Event$ Untap | ValidCard$ Card.Self | Layer$ CantHappen | Description$ CARDNAME doesn't untap during your untap step."), p, engine.Battlefield)
	g.Card(c).Tapped, g.Card(c).SummonSick = true, true

	g.StartTurn(p, engine.NewScriptedController())

	if !g.Card(c).Tapped {
		t.Error("Tapped = false, want true -- Layer$ CantHappen must block the untap")
	}
	if g.Card(c).SummonSick {
		t.Error("SummonSick = true, want false -- a doesn't-untap effect must not block summoning sickness clearing")
	}
}

// TestUntapNotBlockedWhenValidCardDoesNotMatch proves ValidCard$ is checked,
// not assumed: a line naming Land must not stop a Creature from untapping.
func TestUntapNotBlockedWhenValidCardDoesNotMatch(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a")
	p := g.Players()[0]
	c := g.NewCard(replacementCreatureDefPT(t, "Test Unlocked Creature", "2", "2",
		"Event$ Untap | ValidCard$ Land | Layer$ CantHappen | Description$ Lands don't untap during their controller's untap step."), p, engine.Battlefield)
	g.Card(c).Tapped = true

	g.StartTurn(p, engine.NewScriptedController())

	if g.Card(c).Tapped {
		t.Error("Tapped = true, want false -- ValidCard$ Land must not match a Creature")
	}
}

// TestUntapBlockedByOpponentsCantHappenReplacement proves the "other" half
// untapBlocked shares with checkMovedReplacement/checkETBTriggers: a
// permanent on one player's own battlefield can lock an OPPONENT's creature
// out of untapping, ValidCard$ Creature.OppCtrl evaluated relative to the
// lock's own controller.
func TestUntapBlockedByOpponentsCantHappenReplacement(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	a, b := g.Players()[0], g.Players()[1]
	g.NewCard(replacementEnchantmentDef(t, "Test Untap Lock",
		"Event$ Untap | ValidCard$ Creature.OppCtrl | Layer$ CantHappen | Description$ Creatures your opponents control don't untap during their controllers' untap steps."), a, engine.Battlefield)
	c := g.NewCard(replacementCreatureDefPT(t, "Test Opponent Creature", "2", "2"), b, engine.Battlefield)
	g.Card(c).Tapped = true

	g.StartTurn(b, engine.NewScriptedController())

	if !g.Card(c).Tapped {
		t.Error("Tapped = false, want true -- Test Untap Lock's own ValidCard$ Creature.OppCtrl must reach a's opponent's creature")
	}
}

// TestUntapNotBlockedByReplaceWithShape proves a ReplaceWith$-bearing
// Event$ Untap line (2 of the corpus's 158 real lines) is skipped as
// unresolved rather than treated as Layer$ CantHappen: the whole line does
// not apply, so the plain untap happens (PORT-8/GO-7) -- the identical
// "skip means the un-replaced event proceeds" contract
// replacementTapsOnMove already has for an unresolved Moved shape.
func TestUntapNotBlockedByReplaceWithShape(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a")
	p := g.Players()[0]
	reg, err := cardtype.LoadRegistry(strings.NewReader("[CreatureTypes]\nElf\n"))
	if err != nil {
		t.Fatalf("LoadRegistry: %v", err)
	}
	raw := &carddb.Card{Filename: "Test Replace Creature"}
	raw.Faces[0].Present = true
	raw.Faces[0].Name = "Test Replace Creature"
	raw.Faces[0].Type = cardtype.Parse(reg, "Creature Elf")
	raw.Faces[0].Power, raw.Faces[0].Toughness = "2", "2"
	raw.Faces[0].Replacements = []string{"Event$ Untap | ValidCard$ Card.Self | ReplaceWith$ DBUntapTwo | Description$ untaps two things instead."}
	raw.Faces[0].SVars.Set("DBUntapTwo", "DB$ Cleanup")
	def, err := compile.Compile(raw)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	c := g.NewCard(def, p, engine.Battlefield)
	g.Card(c).Tapped = true

	g.StartTurn(p, engine.NewScriptedController())

	if g.Card(c).Tapped {
		t.Error("Tapped = true, want false -- a ReplaceWith$ shape is not Layer$ CantHappen, so the plain untap must proceed")
	}
}

// TestUntapBlockedWhenHostInCommandZone proves ActiveZones$ Command (2 of
// the corpus's 156 real Untap|CantHappen lines) is honored: a lock living in
// the Command zone, not Battlefield, still reaches a creature it names.
func TestUntapBlockedWhenHostInCommandZone(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a")
	p := g.Players()[0]
	g.NewCard(replacementEnchantmentDef(t, "Test Command Lock",
		"Event$ Untap | ValidCard$ Card | ActiveZones$ Command | Layer$ CantHappen | Description$ Nothing untaps."), p, engine.Command)
	c := g.NewCard(replacementCreatureDefPT(t, "Test Creature Under Lock", "2", "2"), p, engine.Battlefield)
	g.Card(c).Tapped = true

	g.StartTurn(p, engine.NewScriptedController())

	if !g.Card(c).Tapped {
		t.Error("Tapped = false, want true -- ActiveZones$ Command must still let a Command-zone host's replacement apply")
	}
}

// TestUntapBlockedWhenIsPresentConditionMet proves untapReplacementMatches'
// own general-gate fold-in (replacementRequirementsCheck, replacement.go):
// IsPresent$ Creature.YouCtrl is genuinely met here (the card carrying the
// lock is itself the only creature this player controls), so the
// "doesn't untap" line applies and Tapped stays true -- alirios_enraptured.txt's
// own real "doesn't untap ... if you control a Reflection" shape, just
// checked against a real, present creature rather than an absent one.
func TestUntapBlockedWhenIsPresentConditionMet(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a")
	p := g.Players()[0]
	c := g.NewCard(replacementCreatureDefPT(t, "Test Conditional Lock Creature", "2", "2",
		"Event$ Untap | ValidCard$ Card.Self | Layer$ CantHappen | IsPresent$ Creature.YouCtrl | Description$ conditional lock."), p, engine.Battlefield)
	g.Card(c).Tapped = true

	g.StartTurn(p, engine.NewScriptedController())

	if !g.Card(c).Tapped {
		t.Error("Tapped = false, want true -- IsPresent$ Creature.YouCtrl is met (the card itself is a creature you control), so the lock must apply")
	}
}

// TestUntapNotBlockedWhenIsPresentConditionNotMet is the same lock's own
// mirror: with no matching creature present at all, IsPresent$ is not met,
// so the "doesn't untap" line does not apply and the card untaps normally.
func TestUntapNotBlockedWhenIsPresentConditionNotMet(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a")
	p := g.Players()[0]
	c := g.NewCard(replacementEnchantmentDef(t, "Test Conditional Lock Enchantment",
		"Event$ Untap | ValidCard$ Card.Self | Layer$ CantHappen | IsPresent$ Creature.YouCtrl | Description$ conditional lock."), p, engine.Battlefield)
	g.Card(c).Tapped = true

	g.StartTurn(p, engine.NewScriptedController())

	if g.Card(c).Tapped {
		t.Error("Tapped = true, want false -- IsPresent$ Creature.YouCtrl is not met (an Enchantment is not a Creature), so the lock must not apply")
	}
}

// TestDamageToPlayerPreventedByReplacement proves damagePreventedPlayer's
// own simplest real shape: Event$ DamageDone | ValidTarget$ You |
// Prevent$ True stops combat damage from reaching its controller entirely.
func TestDamageToPlayerPreventedByReplacement(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	a, b := g.Players()[0], g.Players()[1]
	g.Player(b).Life = 20
	g.SetTurnState(1, a, engine.Main1)
	g.NewCard(replacementEnchantmentDef(t, "Test Damage Shield",
		"Event$ DamageDone | ValidTarget$ You | Prevent$ True | Description$ Prevent all damage that would be dealt to you."), b, engine.Battlefield)
	attacker := g.NewCard(creatureDefPT(t, "3", "3"), a, engine.Battlefield)

	ac := engine.NewScriptedController()
	ac.QueueAttackers([]engine.CardID{attacker})
	g.DeclareCombatAttackers(ac)
	bc := engine.NewScriptedController()
	bc.QueueBlocks(nil)
	g.DeclareCombatBlockers(bc)
	g.DealCombatDamage(engine.NewScriptedController())

	if g.Player(b).Life != 20 {
		t.Errorf("defender life = %d, want 20 -- Prevent$ True must stop the damage entirely", g.Player(b).Life)
	}
}

// TestDamageToPlayerNotPreventedWhenValidTargetDoesNotMatch proves
// ValidTarget$ is checked, not assumed: a shield naming Opponent (relative
// to its own controller) must not stop damage dealt to its own controller.
func TestDamageToPlayerNotPreventedWhenValidTargetDoesNotMatch(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	a, b := g.Players()[0], g.Players()[1]
	g.Player(b).Life = 20
	g.SetTurnState(1, a, engine.Main1)
	g.NewCard(replacementEnchantmentDef(t, "Test Wrong Shield",
		"Event$ DamageDone | ValidTarget$ Opponent | Prevent$ True | Description$ Prevent all damage that would be dealt to your opponents."), b, engine.Battlefield)
	attacker := g.NewCard(creatureDefPT(t, "3", "3"), a, engine.Battlefield)

	ac := engine.NewScriptedController()
	ac.QueueAttackers([]engine.CardID{attacker})
	g.DeclareCombatAttackers(ac)
	bc := engine.NewScriptedController()
	bc.QueueBlocks(nil)
	g.DeclareCombatBlockers(bc)
	g.DealCombatDamage(engine.NewScriptedController())

	if g.Player(b).Life != 17 {
		t.Errorf("defender life = %d, want 17 -- ValidTarget$ Opponent must not match b's own damage", g.Player(b).Life)
	}
}

// TestDamageToCreaturePreventedByReplacement proves damagePrevented's own
// Card-target half: a blocker shielding itself with
// Event$ DamageDone | ValidTarget$ Card.Self | Prevent$ True takes no
// combat damage, while the attacker it damages is unaffected.
func TestDamageToCreaturePreventedByReplacement(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	a, b := g.Players()[0], g.Players()[1]
	g.SetTurnState(1, a, engine.Main1)
	attacker := g.NewCard(creatureDefPT(t, "3", "3"), a, engine.Battlefield)
	blocker := g.NewCard(replacementCreatureDefPT(t, "Test Shielded Blocker", "2", "2",
		"Event$ DamageDone | ValidTarget$ Card.Self | Prevent$ True | Description$ Prevent all damage that would be dealt to CARDNAME."), b, engine.Battlefield)

	ac := engine.NewScriptedController()
	ac.QueueAttackers([]engine.CardID{attacker})
	g.DeclareCombatAttackers(ac)
	bc := engine.NewScriptedController()
	bc.QueueBlocks([]engine.Block{{Blocker: blocker, Attacker: attacker}})
	g.DeclareCombatBlockers(bc)
	g.DealCombatDamage(engine.NewScriptedController())

	if g.Card(blocker).Damage.Marked != 0 {
		t.Errorf("blocker damage marked = %d, want 0 -- Prevent$ True must stop damage to the blocker", g.Card(blocker).Damage.Marked)
	}
	if g.Card(attacker).Damage.Marked != 2 {
		t.Errorf("attacker damage marked = %d, want 2 -- the blocker's own shield must not stop the attacker from taking damage", g.Card(attacker).Damage.Marked)
	}
}

// TestDamageToCreatureNotPreventedWhenValidSourceDoesNotMatch proves
// ValidSource$ is checked, not assumed: a shield naming Dragon must not stop
// damage from a non-Dragon attacker.
func TestDamageToCreatureNotPreventedWhenValidSourceDoesNotMatch(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	a, b := g.Players()[0], g.Players()[1]
	g.SetTurnState(1, a, engine.Main1)
	attacker := g.NewCard(creatureDefPT(t, "3", "3"), a, engine.Battlefield)
	blocker := g.NewCard(replacementCreatureDefPT(t, "Test Wrongly Shielded Blocker", "2", "2",
		"Event$ DamageDone | ValidTarget$ Card.Self | ValidSource$ Dragon | Prevent$ True | Description$ Prevent all damage that would be dealt to CARDNAME by Dragons."), b, engine.Battlefield)

	ac := engine.NewScriptedController()
	ac.QueueAttackers([]engine.CardID{attacker})
	g.DeclareCombatAttackers(ac)
	bc := engine.NewScriptedController()
	bc.QueueBlocks([]engine.Block{{Blocker: blocker, Attacker: attacker}})
	g.DeclareCombatBlockers(bc)
	g.DealCombatDamage(engine.NewScriptedController())

	if g.Card(blocker).Damage.Marked != 3 {
		t.Errorf("blocker damage marked = %d, want 3 -- ValidSource$ Dragon must not match a non-Dragon attacker", g.Card(blocker).Damage.Marked)
	}
}

// TestDamageToCreatureNotPreventedByUndefinedCheckSVar proves
// CheckSVar$/SVarCompare$ now resolve generically through
// replacementRequirementsCheck (triggerCommonRequirementsMet's own
// checkSVarMatches) rather than being an outright-rejected extra param --
// but a card naming CheckSVar$ X with no SVar:X of its own still fails
// closed (checkSVarMatches' own resolveNamedAmount failure returns false,
// "condition not met," not "guess the shape and reject the line"): the
// shield still does not apply, for the honest reason of an unresolvable
// reference rather than an unrecognized param name.
func TestDamageToCreatureNotPreventedByUndefinedCheckSVar(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	a, b := g.Players()[0], g.Players()[1]
	g.SetTurnState(1, a, engine.Main1)
	attacker := g.NewCard(creatureDefPT(t, "3", "3"), a, engine.Battlefield)
	blocker := g.NewCard(replacementCreatureDefPT(t, "Test Conditionally Shielded Blocker", "2", "2",
		"Event$ DamageDone | ValidTarget$ Card.Self | CheckSVar$ X | Prevent$ True | Description$ conditional shield."), b, engine.Battlefield)

	ac := engine.NewScriptedController()
	ac.QueueAttackers([]engine.CardID{attacker})
	g.DeclareCombatAttackers(ac)
	bc := engine.NewScriptedController()
	bc.QueueBlocks([]engine.Block{{Blocker: blocker, Attacker: attacker}})
	g.DeclareCombatBlockers(bc)
	g.DealCombatDamage(engine.NewScriptedController())

	if g.Card(blocker).Damage.Marked != 3 {
		t.Errorf("blocker damage marked = %d, want 3 -- CheckSVar$ X has no SVar:X on this card, so it cannot resolve and the shield must not apply", g.Card(blocker).Damage.Marked)
	}
}

// TestDamageToPlayerPreventedWhenPlayerTurnMatches proves PlayerTurn$ True
// resolving through replacementRequirementsCheck (guardian_naga_banishing_coils.txt's
// own real "can't be dealt damage during your turn" shape): a shield naming
// PlayerTurn$ True stops damage to its own controller during ITS OWN turn.
func TestDamageToPlayerPreventedWhenPlayerTurnMatches(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	a, b := g.Players()[0], g.Players()[1]
	g.Player(a).Life = 20
	g.SetTurnState(1, a, engine.Main1)
	g.NewCard(replacementEnchantmentDef(t, "Test Turn-Locked Shield",
		"Event$ DamageDone | ValidTarget$ You | PlayerTurn$ True | Prevent$ True | Description$ Prevent all damage that would be dealt to you during your turn."), a, engine.Battlefield)
	attacker := g.NewCard(creatureDefPT(t, "3", "3"), b, engine.Battlefield)

	ac := engine.NewScriptedController()
	ac.QueueAttackers([]engine.CardID{attacker})
	g.DeclareCombatAttackers(ac)
	bc := engine.NewScriptedController()
	bc.QueueBlocks(nil)
	g.DeclareCombatBlockers(bc)
	g.DealCombatDamage(engine.NewScriptedController())

	if g.Player(a).Life != 20 {
		t.Errorf("a life = %d, want 20 -- PlayerTurn$ True is met on a's own turn, so the shield must stop the damage", g.Player(a).Life)
	}
}

// TestDamageToPlayerNotPreventedWhenPlayerTurnDoesNotMatch is the same
// shield's own mirror: on b's turn (not a's own), PlayerTurn$ True is not
// met and the shield does not apply.
func TestDamageToPlayerNotPreventedWhenPlayerTurnDoesNotMatch(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	a, b := g.Players()[0], g.Players()[1]
	g.Player(a).Life = 20
	g.SetTurnState(1, b, engine.Main1)
	g.NewCard(replacementEnchantmentDef(t, "Test Turn-Locked Shield",
		"Event$ DamageDone | ValidTarget$ You | PlayerTurn$ True | Prevent$ True | Description$ Prevent all damage that would be dealt to you during your turn."), a, engine.Battlefield)
	attacker := g.NewCard(creatureDefPT(t, "3", "3"), b, engine.Battlefield)

	ac := engine.NewScriptedController()
	ac.QueueAttackers([]engine.CardID{attacker})
	g.DeclareCombatAttackers(ac)
	bc := engine.NewScriptedController()
	bc.QueueBlocks(nil)
	g.DeclareCombatBlockers(bc)
	g.DealCombatDamage(engine.NewScriptedController())

	if g.Player(a).Life != 17 {
		t.Errorf("a life = %d, want 17 -- PlayerTurn$ True must not apply on b's turn (not a's own)", g.Player(a).Life)
	}
}
