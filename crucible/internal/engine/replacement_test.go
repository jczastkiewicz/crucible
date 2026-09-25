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

// TestDamageToPlayerReducedByReplaceDamage proves damageReplaced/
// damageReplacedPlayer (replacement.go) resolve orbs_of_warding.txt's own
// real shape: DB$ ReplaceDamage | Amount$ 1 reduces a 3-damage combat hit to
// 2 rather than skipping the event outright -- CR 616's own "Updated"
// outcome, distinct from Prevent$ True's "the event does not happen at all".
func TestDamageToPlayerReducedByReplaceDamage(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	a, b := g.Players()[0], g.Players()[1]
	g.Player(b).Life = 20
	g.SetTurnState(1, a, engine.Main1)
	g.NewCard(replacementEnchantmentDefWithSVar(t, "Test Orbs of Warding",
		"Event$ DamageDone | ValidTarget$ You | ReplaceWith$ DBReplace | Description$ Prevent 1 of that damage.",
		"DBReplace", "DB$ ReplaceDamage | Amount$ 1"), b, engine.Battlefield)
	attacker := g.NewCard(creatureDefPT(t, "3", "3"), a, engine.Battlefield)

	ac := engine.NewScriptedController()
	ac.QueueAttackers([]engine.CardID{attacker})
	g.DeclareCombatAttackers(ac)
	bc := engine.NewScriptedController()
	bc.QueueBlocks(nil)
	g.DeclareCombatBlockers(bc)
	g.DealCombatDamage(engine.NewScriptedController())

	if g.Player(b).Life != 18 {
		t.Errorf("defender life = %d, want 18 -- 3 damage minus the shield's own Amount$ 1 leaves 2", g.Player(b).Life)
	}
}

// TestDamageToPlayerReductionClampsAtZero proves a reduction bigger than the
// incoming damage stops at zero rather than going negative -- CR 616's own
// "Replaced" outcome once the reduction reaches zero or below, folded into
// the identical "nothing happens" damagePrevented already gives.
func TestDamageToPlayerReductionClampsAtZero(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	a, b := g.Players()[0], g.Players()[1]
	g.Player(b).Life = 20
	g.SetTurnState(1, a, engine.Main1)
	g.NewCard(replacementEnchantmentDefWithSVar(t, "Test Shield of the Realm",
		"Event$ DamageDone | ValidTarget$ You | ReplaceWith$ DBReplace | Description$ Prevent 5 of that damage.",
		"DBReplace", "DB$ ReplaceDamage | Amount$ 5"), b, engine.Battlefield)
	attacker := g.NewCard(creatureDefPT(t, "3", "3"), a, engine.Battlefield)

	ac := engine.NewScriptedController()
	ac.QueueAttackers([]engine.CardID{attacker})
	g.DeclareCombatAttackers(ac)
	bc := engine.NewScriptedController()
	bc.QueueBlocks(nil)
	g.DeclareCombatBlockers(bc)
	g.DealCombatDamage(engine.NewScriptedController())

	if g.Player(b).Life != 20 {
		t.Errorf("defender life = %d, want 20 -- reducing 3 damage by 5 must clamp at 0, not go negative", g.Player(b).Life)
	}
}

// replacementCreatureDefPTWithSVar is replacementCreatureDefPT's own
// SVar-carrying sibling -- damageReplaced's own Card-target tests need both
// a real R: line and the SVar its own ReplaceWith$ names, the identical
// pairing replacementEnchantmentDefWithSVar (drawreplaced_test.go) already
// gives a non-creature host.
func replacementCreatureDefPTWithSVar(t *testing.T, name, power, toughness, replacement, svarName, svarBody string) *compile.Card {
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
	raw.Faces[0].Replacements = []string{replacement}
	raw.Faces[0].SVars.Set(svarName, svarBody)

	c, err := compile.Compile(raw)
	if err != nil {
		t.Fatalf("compile %q: %v", name, err)
	}
	return c
}

// replacementCreatureDefPTWithSVars is replacementCreatureDefPTWithSVar's own
// multi-SVar sibling, the identical reason replacementEnchantmentDefWithSVars
// (below) is replacementEnchantmentDefWithSVar's.
func replacementCreatureDefPTWithSVars(t *testing.T, name, power, toughness, replacement string, svars map[string]string) *compile.Card {
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
	raw.Faces[0].Replacements = []string{replacement}
	for svarName, svarBody := range svars {
		raw.Faces[0].SVars.Set(svarName, svarBody)
	}

	c, err := compile.Compile(raw)
	if err != nil {
		t.Fatalf("compile %q: %v", name, err)
	}
	return c
}

// TestDamageToCreatureReducedByReplaceDamage proves damageReplaced's own
// Card-target half: a blocker reducing incoming damage by 1 (shield_of_the_
// realm.txt's own "prevent 2 of that damage dealt to equipped creature"
// shape, simplified to Card.Self the identical way
// TestDamageToCreaturePreventedByReplacement already does for Prevent$ True)
// takes 1 less damage than the attacker's own power.
func TestDamageToCreatureReducedByReplaceDamage(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	a, b := g.Players()[0], g.Players()[1]
	g.SetTurnState(1, a, engine.Main1)
	attacker := g.NewCard(creatureDefPT(t, "3", "3"), a, engine.Battlefield)
	blocker := g.NewCard(replacementCreatureDefPTWithSVar(t, "Test Shielded Blocker", "2", "2",
		"Event$ DamageDone | ValidTarget$ Card.Self | ReplaceWith$ DBReplace | Description$ Prevent 1 of that damage.",
		"DBReplace", "DB$ ReplaceDamage | Amount$ 1"), b, engine.Battlefield)

	ac := engine.NewScriptedController()
	ac.QueueAttackers([]engine.CardID{attacker})
	g.DeclareCombatAttackers(ac)
	bc := engine.NewScriptedController()
	bc.QueueBlocks([]engine.Block{{Blocker: blocker, Attacker: attacker}})
	g.DeclareCombatBlockers(bc)
	g.DealCombatDamage(engine.NewScriptedController())

	if g.Card(blocker).Damage.Marked != 2 {
		t.Errorf("blocker damage marked = %d, want 2 -- 3 damage minus the shield's own Amount$ 1 leaves 2", g.Card(blocker).Damage.Marked)
	}
}

// TestDamageToCreatureNotReducedByChainedSubAbility proves a ReplaceWith$
// target naming its own SubAbility$ is refused outright rather than run with
// the chained half silently dropped (GO-7): the full 3 damage still applies.
func TestDamageToCreatureNotReducedByChainedSubAbility(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	a, b := g.Players()[0], g.Players()[1]
	g.SetTurnState(1, a, engine.Main1)
	attacker := g.NewCard(creatureDefPT(t, "3", "3"), a, engine.Battlefield)
	raw := &carddb.Card{Filename: "Test Wrongly Shielded Blocker"}
	raw.Faces[0].Present = true
	raw.Faces[0].Name = "Test Wrongly Shielded Blocker"
	raw.Faces[0].Type = cardtype.Parse(attachmentTypeRegistry(t), "Creature Elf")
	raw.Faces[0].Power, raw.Faces[0].Toughness = "2", "2"
	raw.Faces[0].Replacements = []string{
		"Event$ DamageDone | ValidTarget$ Card.Self | ReplaceWith$ DBReplace | Description$ Prevent 1 of that damage, then draw a card.",
	}
	raw.Faces[0].SVars.Set("DBReplace", "DB$ ReplaceDamage | Amount$ 1 | SubAbility$ DBDraw")
	raw.Faces[0].SVars.Set("DBDraw", "DB$ Draw | Defined$ You | NumCards$ 1")
	def, err := compile.Compile(raw)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	blocker := g.NewCard(def, b, engine.Battlefield)

	ac := engine.NewScriptedController()
	ac.QueueAttackers([]engine.CardID{attacker})
	g.DeclareCombatAttackers(ac)
	bc := engine.NewScriptedController()
	bc.QueueBlocks([]engine.Block{{Blocker: blocker, Attacker: attacker}})
	g.DeclareCombatBlockers(bc)
	g.DealCombatDamage(engine.NewScriptedController())

	if g.Card(blocker).Damage.Marked != 3 {
		t.Errorf("blocker damage marked = %d, want 3 -- a chained ReplaceWith$ target must not dispatch, so the full damage must apply", g.Card(blocker).Damage.Marked)
	}
}

// TestDamageToPlayerNotReducedByDivideShieldAmount proves DivideShield$ (a
// shield's own remaining capacity split across more than one simultaneous
// instance of damage) refuses outright rather than applying the reduction
// without tracking that split (GO-7): the full 3 damage still applies.
func TestDamageToPlayerNotReducedByDivideShieldAmount(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	a, b := g.Players()[0], g.Players()[1]
	g.Player(b).Life = 20
	g.SetTurnState(1, a, engine.Main1)
	g.NewCard(replacementEnchantmentDefWithSVar(t, "Test Divided Shield",
		"Event$ DamageDone | ValidTarget$ You | ReplaceWith$ DBReplace | Description$ Prevent damage, divided as you choose.",
		"DBReplace", "DB$ ReplaceDamage | Amount$ 3 | DivideShield$ True"), b, engine.Battlefield)
	attacker := g.NewCard(creatureDefPT(t, "3", "3"), a, engine.Battlefield)

	ac := engine.NewScriptedController()
	ac.QueueAttackers([]engine.CardID{attacker})
	g.DeclareCombatAttackers(ac)
	bc := engine.NewScriptedController()
	bc.QueueBlocks(nil)
	g.DeclareCombatBlockers(bc)
	g.DealCombatDamage(engine.NewScriptedController())

	if g.Player(b).Life != 17 {
		t.Errorf("defender life = %d, want 17 -- DivideShield$ is not resolved, so the full 3 damage must apply", g.Player(b).Life)
	}
}

// replacementEnchantmentDefWithSVars is replacementEnchantmentDefWithSVar's
// own multi-SVar sibling (drawreplaced_test.go) -- a DB$ ReplaceEffect line's
// own VarValue$ names a second SVar (the ReplaceCount$DamageAmount/<op>
// expression) rather than carrying the amount inline the way DB$
// ReplaceDamage's own Amount$ does, so these tests need two.
func replacementEnchantmentDefWithSVars(t *testing.T, name, replacement string, svars map[string]string) *compile.Card {
	t.Helper()

	reg, err := cardtype.LoadRegistry(strings.NewReader("[CreatureTypes]\nElf\n"))
	if err != nil {
		t.Fatalf("LoadRegistry: %v", err)
	}
	raw := &carddb.Card{Filename: name}
	raw.Faces[0].Present = true
	raw.Faces[0].Name = name
	raw.Faces[0].Type = cardtype.Parse(reg, "Enchantment")
	raw.Faces[0].Replacements = []string{replacement}
	for svarName, svarBody := range svars {
		raw.Faces[0].SVars.Set(svarName, svarBody)
	}

	c, err := compile.Compile(raw)
	if err != nil {
		t.Fatalf("compile %q: %v", name, err)
	}
	return c
}

// TestDamageToPlayerDoubledByReplaceEffect proves replaceEffectEffect
// (replaceeffect.go) resolves raphael_the_muscle.txt's/gratuitous_violence.txt's
// own real shape: DB$ ReplaceEffect | VarName$ DamageAmount | VarValue$ X,
// X:ReplaceCount$DamageAmount/Twice doubles a 3-damage combat hit to 6 -- CR
// 616's own "Updated" outcome computing a new amount rather than ReplaceDamage's
// own flat reduction. ValidSource$ dropped and ValidTarget$ simplified to
// You, the shield hosted on the defender itself rather than the attacker's
// controller, so this test isolates the new arithmetic dispatch from
// ValidSource$'s own gate (TestDamageToPlayerNotReducedWhenValidSourceDoesNotMatch,
// below) and from ValidTarget$'s own comma-OR dispatch
// (TestDamageToPlayerDoubledByReplaceEffectWithCommaValidTarget, gratuitous_violence.txt's
// own real Permanent,Player shape, below) -- each proven on its own.
func TestDamageToPlayerDoubledByReplaceEffect(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	a, b := g.Players()[0], g.Players()[1]
	g.Player(b).Life = 20
	g.SetTurnState(1, a, engine.Main1)
	g.NewCard(replacementEnchantmentDefWithSVars(t, "Test Gratuitous Violence",
		"Event$ DamageDone | ValidTarget$ You | ReplaceWith$ DmgTwice | Description$ Double damage.",
		map[string]string{
			"DmgTwice": "DB$ ReplaceEffect | VarName$ DamageAmount | VarValue$ X",
			"X":        "ReplaceCount$DamageAmount/Twice",
		}), b, engine.Battlefield)
	attacker := g.NewCard(creatureDefPT(t, "3", "3"), a, engine.Battlefield)

	ac := engine.NewScriptedController()
	ac.QueueAttackers([]engine.CardID{attacker})
	g.DeclareCombatAttackers(ac)
	bc := engine.NewScriptedController()
	bc.QueueBlocks(nil)
	g.DeclareCombatBlockers(bc)
	g.DealCombatDamage(engine.NewScriptedController())

	if g.Player(b).Life != 14 {
		t.Errorf("defender life = %d, want 14 -- 3 damage doubled to 6", g.Player(b).Life)
	}
}

// TestDamageToPlayerDoubledByReplaceEffectWithCommaValidTarget is
// TestDamageToPlayerDoubledByReplaceEffect's own sibling, proving
// matchesPlayerSpec's own comma-OR split (valid.go) against
// gratuitous_violence.txt's own real, unsimplified ValidTarget$ Permanent,Player:
// "Permanent" is not a player base at all, so it matches nothing for a
// *Player* target and contributes nothing to the OR, but "Player" alone
// (unqualified) matches every player, so the doubling still applies to a
// player-target hit. Before matchesPlayerSpec split on comma, the whole
// string "Permanent,Player" failed matchesPlayerBase outright (it is not the
// literal "You"/"Opponent"/"Player"), so this shield never doubled damage to
// a player at all.
func TestDamageToPlayerDoubledByReplaceEffectWithCommaValidTarget(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	a, b := g.Players()[0], g.Players()[1]
	g.Player(b).Life = 20
	g.SetTurnState(1, a, engine.Main1)
	g.NewCard(replacementEnchantmentDefWithSVars(t, "Test Gratuitous Violence",
		"Event$ DamageDone | ValidTarget$ Permanent,Player | ReplaceWith$ DmgTwice | Description$ Double damage.",
		map[string]string{
			"DmgTwice": "DB$ ReplaceEffect | VarName$ DamageAmount | VarValue$ X",
			"X":        "ReplaceCount$DamageAmount/Twice",
		}), b, engine.Battlefield)
	attacker := g.NewCard(creatureDefPT(t, "3", "3"), a, engine.Battlefield)

	ac := engine.NewScriptedController()
	ac.QueueAttackers([]engine.CardID{attacker})
	g.DeclareCombatAttackers(ac)
	bc := engine.NewScriptedController()
	bc.QueueBlocks(nil)
	g.DeclareCombatBlockers(bc)
	g.DealCombatDamage(engine.NewScriptedController())

	if g.Player(b).Life != 14 {
		t.Errorf("defender life = %d, want 14 -- ValidTarget$ Permanent,Player's own Player alternative must still double damage dealt to a player", g.Player(b).Life)
	}
}

// TestDamageToPlayerReducedByReplaceDamageWithCommaValidTarget proves the
// identical comma-OR split against reidane_god_of_the_worthy_valkmira_protectors_shield.txt's
// own real ValidTarget$ You,Permanent.YouCtrl for ReplaceDamage's own flat
// reduction, not just ReplaceEffect's own computed one, above: "You" (the
// shield's own controller, b) matches the player-target hit directly, so the
// shield still prevents 1 of the 3 damage even though the OR's other half
// (Permanent.YouCtrl) is a card-shaped alternative that can never match a
// player at all.
func TestDamageToPlayerReducedByReplaceDamageWithCommaValidTarget(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	a, b := g.Players()[0], g.Players()[1]
	g.Player(b).Life = 20
	g.SetTurnState(1, a, engine.Main1)
	g.NewCard(replacementEnchantmentDefWithSVar(t, "Test Reidane's Shield",
		"Event$ DamageDone | ActiveZones$ Battlefield | ValidSource$ Card.OppCtrl | ValidTarget$ You,Permanent.YouCtrl | ReplaceWith$ DBReplace | Description$ Prevent 1 damage to you or a permanent you control.",
		"DBReplace", "DB$ ReplaceDamage | Amount$ 1"), b, engine.Battlefield)
	attacker := g.NewCard(creatureDefPT(t, "3", "3"), a, engine.Battlefield)

	ac := engine.NewScriptedController()
	ac.QueueAttackers([]engine.CardID{attacker})
	g.DeclareCombatAttackers(ac)
	bc := engine.NewScriptedController()
	bc.QueueBlocks(nil)
	g.DeclareCombatBlockers(bc)
	g.DealCombatDamage(engine.NewScriptedController())

	if g.Player(b).Life != 18 {
		t.Errorf("defender life = %d, want 18 -- ValidTarget$ You,Permanent.YouCtrl's own You alternative must still prevent 1 of the 3 damage dealt to a player", g.Player(b).Life)
	}
}

// TestDamageToCreatureTripledByReplaceEffect proves replaceEffectEffect's
// own card-target half (city_on_fire.txt's own real Thrice shape, applied to
// a creature the way TestDamageToCreatureReducedByReplaceDamage already
// proves ReplaceDamage's own card-target half).
func TestDamageToCreatureTripledByReplaceEffect(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	a, b := g.Players()[0], g.Players()[1]
	g.SetTurnState(1, a, engine.Main1)
	attacker := g.NewCard(creatureDefPT(t, "2", "2"), a, engine.Battlefield)
	blocker := g.NewCard(replacementCreatureDefPTWithSVars(t, "Test Tripling Wall", "1", "10",
		"Event$ DamageDone | ValidTarget$ Card.Self | ReplaceWith$ DmgTriple | Description$ Triple damage to this.",
		map[string]string{
			"DmgTriple": "DB$ ReplaceEffect | VarName$ DamageAmount | VarValue$ Z",
			"Z":         "ReplaceCount$DamageAmount/Thrice",
		}), b, engine.Battlefield)

	ac := engine.NewScriptedController()
	ac.QueueAttackers([]engine.CardID{attacker})
	g.DeclareCombatAttackers(ac)
	bc := engine.NewScriptedController()
	bc.QueueBlocks([]engine.Block{{Blocker: blocker, Attacker: attacker}})
	g.DeclareCombatBlockers(bc)
	g.DealCombatDamage(engine.NewScriptedController())

	if g.Card(blocker).Damage.Marked != 6 {
		t.Errorf("blocker damage marked = %d, want 6 -- 2 damage tripled", g.Card(blocker).Damage.Marked)
	}
}

// TestDamageToPlayerNotReducedWhenValidSourceDoesNotMatch proves
// ValidSource$ is checked for the ReplaceWith$ shape the identical way
// TestDamageToCreatureNotPreventedWhenValidSourceDoesNotMatch already proves
// it for Prevent$ True: a shield naming Dragon must not reduce damage from a
// non-Dragon attacker.
func TestDamageToPlayerNotReducedWhenValidSourceDoesNotMatch(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	a, b := g.Players()[0], g.Players()[1]
	g.Player(b).Life = 20
	g.SetTurnState(1, a, engine.Main1)
	g.NewCard(replacementEnchantmentDefWithSVar(t, "Test Wrongly Sourced Shield",
		"Event$ DamageDone | ValidTarget$ You | ValidSource$ Dragon | ReplaceWith$ DBReplace | Description$ Prevent 1 of that damage from Dragons.",
		"DBReplace", "DB$ ReplaceDamage | Amount$ 1"), b, engine.Battlefield)
	attacker := g.NewCard(creatureDefPT(t, "3", "3"), a, engine.Battlefield)

	ac := engine.NewScriptedController()
	ac.QueueAttackers([]engine.CardID{attacker})
	g.DeclareCombatAttackers(ac)
	bc := engine.NewScriptedController()
	bc.QueueBlocks(nil)
	g.DeclareCombatBlockers(bc)
	g.DealCombatDamage(engine.NewScriptedController())

	if g.Player(b).Life != 17 {
		t.Errorf("defender life = %d, want 17 -- ValidSource$ Dragon must not match a non-Dragon attacker", g.Player(b).Life)
	}
}

// TestDamageToPlayerReplaceEffectPlusLiteral proves the Plus operator with a
// literal operand -- torbran_thane_of_red_fell.txt's own real
// ReplaceCount$DamageAmount/Plus.2 shape -- adds rather than scales: 3
// damage becomes 5, not 6. ValidSource$ dropped and the shield hosted on the
// defender, the identical simplification
// TestDamageToPlayerDoubledByReplaceEffect's own doc comment gives.
func TestDamageToPlayerReplaceEffectPlusLiteral(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	a, b := g.Players()[0], g.Players()[1]
	g.Player(b).Life = 20
	g.SetTurnState(1, a, engine.Main1)
	g.NewCard(replacementEnchantmentDefWithSVars(t, "Test Torbran",
		"Event$ DamageDone | ValidTarget$ You | ReplaceWith$ DmgPlus2 | Description$ Plus 2 damage.",
		map[string]string{
			"DmgPlus2": "DB$ ReplaceEffect | VarName$ DamageAmount | VarValue$ X",
			"X":        "ReplaceCount$DamageAmount/Plus.2",
		}), b, engine.Battlefield)
	attacker := g.NewCard(creatureDefPT(t, "3", "3"), a, engine.Battlefield)

	ac := engine.NewScriptedController()
	ac.QueueAttackers([]engine.CardID{attacker})
	g.DeclareCombatAttackers(ac)
	bc := engine.NewScriptedController()
	bc.QueueBlocks(nil)
	g.DeclareCombatBlockers(bc)
	g.DealCombatDamage(engine.NewScriptedController())

	if g.Player(b).Life != 15 {
		t.Errorf("defender life = %d, want 15 -- 3 damage plus 2 leaves 15", g.Player(b).Life)
	}
}

// TestDamageToPlayerReplaceEffectMinusClampsAtZero proves the Minus operator
// -- benevolent_unicorn.txt's/lashknife_barrier.txt's own real
// ReplaceCount$DamageAmount/Minus.1 shape -- and that a reduction to zero
// applies no damage at all, the identical clamp replaceDamageEffect's
// own Amount$ reduction already gives. ValidSource$ Spell dropped (the real
// card restricts to spell-dealt damage; this test drives the shape through
// combat instead, the same reason every other test in this file simplifies
// away a param it is not exercising) and the shield hosted on the defender,
// the identical simplification TestDamageToPlayerDoubledByReplaceEffect's
// own doc comment gives.
func TestDamageToPlayerReplaceEffectMinusClampsAtZero(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	a, b := g.Players()[0], g.Players()[1]
	g.Player(b).Life = 20
	g.SetTurnState(1, a, engine.Main1)
	g.NewCard(replacementEnchantmentDefWithSVars(t, "Test Benevolent Unicorn",
		"Event$ DamageDone | ValidTarget$ You | ReplaceWith$ DmgMinus1 | Description$ Minus 1 damage.",
		map[string]string{
			"DmgMinus1": "DB$ ReplaceEffect | VarName$ DamageAmount | VarValue$ X",
			"X":         "ReplaceCount$DamageAmount/Minus.1",
		}), b, engine.Battlefield)
	attacker := g.NewCard(creatureDefPT(t, "1", "1"), a, engine.Battlefield)

	ac := engine.NewScriptedController()
	ac.QueueAttackers([]engine.CardID{attacker})
	g.DeclareCombatAttackers(ac)
	bc := engine.NewScriptedController()
	bc.QueueBlocks(nil)
	g.DeclareCombatBlockers(bc)
	g.DealCombatDamage(engine.NewScriptedController())

	if g.Player(b).Life != 20 {
		t.Errorf("defender life = %d, want 20 -- 1 damage minus 1 must clamp at 0, not go negative", g.Player(b).Life)
	}
}

// TestDamageToPlayerReplaceEffectHalfDown proves the HalfDown operator --
// ghosts_of_the_innocent.txt's own real ReplaceCount$DamageAmount/HalfDown
// shape -- rounds down rather than up: 3 damage becomes 1, not 2. The
// shield hosted on the defender, the identical simplification
// TestDamageToPlayerDoubledByReplaceEffect's own doc comment gives.
func TestDamageToPlayerReplaceEffectHalfDown(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	a, b := g.Players()[0], g.Players()[1]
	g.Player(b).Life = 20
	g.SetTurnState(1, a, engine.Main1)
	g.NewCard(replacementEnchantmentDefWithSVars(t, "Test Ghosts of the Innocent",
		"Event$ DamageDone | ValidTarget$ You | ReplaceWith$ DmgHalf | Description$ Half damage, rounded down.",
		map[string]string{
			"DmgHalf": "DB$ ReplaceEffect | VarName$ DamageAmount | VarValue$ X",
			"X":       "ReplaceCount$DamageAmount/HalfDown",
		}), b, engine.Battlefield)
	attacker := g.NewCard(creatureDefPT(t, "3", "3"), a, engine.Battlefield)

	ac := engine.NewScriptedController()
	ac.QueueAttackers([]engine.CardID{attacker})
	g.DeclareCombatAttackers(ac)
	bc := engine.NewScriptedController()
	bc.QueueBlocks(nil)
	g.DeclareCombatBlockers(bc)
	g.DealCombatDamage(engine.NewScriptedController())

	if g.Player(b).Life != 19 {
		t.Errorf("defender life = %d, want 19 -- 3 damage halved and rounded down leaves 1, so life drops by 1", g.Player(b).Life)
	}
}

// TestDamageToPlayerReplaceEffectFlatReplacementAboveGate proves a plain
// integer VarValue$ (forethought_amulet.txt's/divine_presence.txt's own real
// shape) replaces the amount outright, gated by the R: line's own
// DamageAmount$ threshold matching the ORIGINAL amount: 3 damage (>= the
// line's own GE3) becomes 2.
func TestDamageToPlayerReplaceEffectFlatReplacementAboveGate(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	a, b := g.Players()[0], g.Players()[1]
	g.Player(b).Life = 20
	g.SetTurnState(1, a, engine.Main1)
	g.NewCard(replacementEnchantmentDefWithSVar(t, "Test Forethought Amulet",
		"Event$ DamageDone | ValidTarget$ You | DamageAmount$ GE3 | ReplaceWith$ Dmg2 | Description$ If 3 or more damage, deal 2 instead.",
		"Dmg2", "DB$ ReplaceEffect | VarName$ DamageAmount | VarValue$ 2"), b, engine.Battlefield)
	attacker := g.NewCard(creatureDefPT(t, "3", "3"), a, engine.Battlefield)

	ac := engine.NewScriptedController()
	ac.QueueAttackers([]engine.CardID{attacker})
	g.DeclareCombatAttackers(ac)
	bc := engine.NewScriptedController()
	bc.QueueBlocks(nil)
	g.DeclareCombatBlockers(bc)
	g.DealCombatDamage(engine.NewScriptedController())

	if g.Player(b).Life != 18 {
		t.Errorf("defender life = %d, want 18 -- 3 damage meets the GE3 gate and is replaced with 2", g.Player(b).Life)
	}
}

// TestDamageToPlayerReplaceEffectFlatReplacementBelowGateNotApplied proves
// the same DamageAmount$ GE3 gate refuses to apply when the original amount
// falls short. 1 damage, not 2, so the outcome would be observably different
// (2 damage instead of 1) if the gate were ignored -- unlike a 2-damage
// attacker, which coincidentally matches this line's own flat VarValue$ 2
// either way and could never distinguish "correctly gated" from "always
// fires".
func TestDamageToPlayerReplaceEffectFlatReplacementBelowGateNotApplied(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	a, b := g.Players()[0], g.Players()[1]
	g.Player(b).Life = 20
	g.SetTurnState(1, a, engine.Main1)
	g.NewCard(replacementEnchantmentDefWithSVar(t, "Test Forethought Amulet",
		"Event$ DamageDone | ValidTarget$ You | DamageAmount$ GE3 | ReplaceWith$ Dmg2 | Description$ If 3 or more damage, deal 2 instead.",
		"Dmg2", "DB$ ReplaceEffect | VarName$ DamageAmount | VarValue$ 2"), b, engine.Battlefield)
	attacker := g.NewCard(creatureDefPT(t, "1", "1"), a, engine.Battlefield)

	ac := engine.NewScriptedController()
	ac.QueueAttackers([]engine.CardID{attacker})
	g.DeclareCombatAttackers(ac)
	bc := engine.NewScriptedController()
	bc.QueueBlocks(nil)
	g.DeclareCombatBlockers(bc)
	g.DealCombatDamage(engine.NewScriptedController())

	if g.Player(b).Life != 19 {
		t.Errorf("defender life = %d, want 19 -- 1 damage does not meet the GE3 gate, so it must apply unchanged", g.Player(b).Life)
	}
}

// TestDamageToPlayerReplaceEffectUnresolvedOperandNotApplied proves a Plus
// operand this port cannot resolve (hawkeye_young_avenger.txt's own real
// Plus.Y, Y:Count$CardPower -- neither the Valid family resolveAmount
// evaluates) refuses outright (GO-7) rather than treating the unresolved
// operand as zero: the full original damage applies. ValidSource$ dropped
// and the shield hosted on the defender, the identical simplification
// TestDamageToPlayerDoubledByReplaceEffect's own doc comment gives.
func TestDamageToPlayerReplaceEffectUnresolvedOperandNotApplied(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	a, b := g.Players()[0], g.Players()[1]
	g.Player(b).Life = 20
	g.SetTurnState(1, a, engine.Main1)
	g.NewCard(replacementEnchantmentDefWithSVars(t, "Test Hawkeye",
		"Event$ DamageDone | ValidTarget$ You | ReplaceWith$ DmgPlusX | Description$ Plus X damage.",
		map[string]string{
			"DmgPlusX": "DB$ ReplaceEffect | VarName$ DamageAmount | VarValue$ X",
			"X":        "ReplaceCount$DamageAmount/Plus.Y",
			"Y":        "Count$CardPower",
		}), b, engine.Battlefield)
	attacker := g.NewCard(creatureDefPT(t, "3", "3"), a, engine.Battlefield)

	ac := engine.NewScriptedController()
	ac.QueueAttackers([]engine.CardID{attacker})
	g.DeclareCombatAttackers(ac)
	bc := engine.NewScriptedController()
	bc.QueueBlocks(nil)
	g.DeclareCombatBlockers(bc)
	g.DealCombatDamage(engine.NewScriptedController())

	if g.Player(b).Life != 17 {
		t.Errorf("defender life = %d, want 17 -- an unresolvable Plus operand must not apply, leaving the plain 3 damage", g.Player(b).Life)
	}
}

// TestDamageToCreaturePreventedWhenDamageAmountGateMatches proves
// damagePreventionMatches' own new DamageAmount$ gate (callous_giant.txt's
// own real "if a source would deal 3 or less damage to CARDNAME, prevent
// that damage," Prevent$ True | DamageAmount$ LE3): a 3-damage hit meets the
// gate and is fully prevented.
func TestDamageToCreaturePreventedWhenDamageAmountGateMatches(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	a, b := g.Players()[0], g.Players()[1]
	g.SetTurnState(1, a, engine.Main1)
	attacker := g.NewCard(creatureDefPT(t, "3", "3"), a, engine.Battlefield)
	blocker := g.NewCard(replacementCreatureDefPT(t, "Test Callous Giant", "4", "4",
		"Event$ DamageDone | ValidTarget$ Card.Self | DamageAmount$ LE3 | Prevent$ True | Description$ If 3 or less damage, prevent it."), b, engine.Battlefield)

	ac := engine.NewScriptedController()
	ac.QueueAttackers([]engine.CardID{attacker})
	g.DeclareCombatAttackers(ac)
	bc := engine.NewScriptedController()
	bc.QueueBlocks([]engine.Block{{Blocker: blocker, Attacker: attacker}})
	g.DeclareCombatBlockers(bc)
	g.DealCombatDamage(engine.NewScriptedController())

	if g.Card(blocker).Damage.Marked != 0 {
		t.Errorf("blocker damage marked = %d, want 0 -- 3 damage meets the LE3 gate and must be fully prevented", g.Card(blocker).Damage.Marked)
	}
}

// TestDamageToCreatureNotPreventedWhenDamageAmountExceedsGate proves the
// same LE3 gate does NOT prevent a bigger hit: 4 damage exceeds it, so the
// full amount applies.
func TestDamageToCreatureNotPreventedWhenDamageAmountExceedsGate(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	a, b := g.Players()[0], g.Players()[1]
	g.SetTurnState(1, a, engine.Main1)
	attacker := g.NewCard(creatureDefPT(t, "4", "4"), a, engine.Battlefield)
	blocker := g.NewCard(replacementCreatureDefPT(t, "Test Callous Giant", "5", "5",
		"Event$ DamageDone | ValidTarget$ Card.Self | DamageAmount$ LE3 | Prevent$ True | Description$ If 3 or less damage, prevent it."), b, engine.Battlefield)

	ac := engine.NewScriptedController()
	ac.QueueAttackers([]engine.CardID{attacker})
	g.DeclareCombatAttackers(ac)
	bc := engine.NewScriptedController()
	bc.QueueBlocks([]engine.Block{{Blocker: blocker, Attacker: attacker}})
	g.DeclareCombatBlockers(bc)
	g.DealCombatDamage(engine.NewScriptedController())

	if g.Card(blocker).Damage.Marked != 4 {
		t.Errorf("blocker damage marked = %d, want 4 -- 4 damage exceeds the LE3 gate, so it must apply in full", g.Card(blocker).Damage.Marked)
	}
}

// equipmentDefWithReplacementSVars builds an Equipment carrying a real R:
// line and the SVars its own ReplaceWith$ chain names -- panther_habit.txt's
// own real "if equipped creature would be dealt damage... put that many
// +1/+1 counters on it" needs an actual attachment link, unlike every other
// applyDamageReplaceCounter test in this file.
func equipmentDefWithReplacementSVars(t *testing.T, name, replacement string, svars map[string]string) *compile.Card {
	t.Helper()

	reg, err := cardtype.LoadRegistry(strings.NewReader("[CreatureTypes]\nElf\n"))
	if err != nil {
		t.Fatalf("LoadRegistry: %v", err)
	}
	raw := &carddb.Card{Filename: name}
	raw.Faces[0].Present = true
	raw.Faces[0].Name = name
	raw.Faces[0].Type = cardtype.Parse(reg, "Artifact Equipment")
	raw.Faces[0].Replacements = []string{replacement}
	for svarName, svarBody := range svars {
		raw.Faces[0].SVars.Set(svarName, svarBody)
	}

	c, err := compile.Compile(raw)
	if err != nil {
		t.Fatalf("compile %q: %v", name, err)
	}
	return c
}

// TestDamageToCreatureReplacedByRemoveCounter proves applyDamageReplaceCounter
// (replacement.go) resolves the "Phantom" family's own real shape --
// unbreathing_horde.txt's/phantom_wurm.txt's/... own "if damage would be
// dealt to CARDNAME, prevent that damage. Remove a +1/+1 counter" -- CR
// 616's own "Replaced" outcome: the damage never happens at all (Damage.Marked
// stays 0), a counter comes off the host instead.
func TestDamageToCreatureReplacedByRemoveCounter(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	a, b := g.Players()[0], g.Players()[1]
	g.SetTurnState(1, a, engine.Main1)
	attacker := g.NewCard(creatureDefPT(t, "3", "3"), a, engine.Battlefield)
	blocker := g.NewCard(replacementCreatureDefPTWithSVar(t, "Test Phantom Wurm", "5", "5",
		"Event$ DamageDone | ActiveZones$ Battlefield | ValidTarget$ Card.Self | ReplaceWith$ DBRemoveCounters | PreventionEffect$ True | AlwaysReplace$ True | Description$ Prevent damage, remove a counter.",
		"DBRemoveCounters", "DB$ RemoveCounter | Defined$ Self | CounterType$ P1P1 | CounterNum$ 1"), b, engine.Battlefield)
	g.Card(blocker).Counters.Add(engine.P1P1, 1)

	ac := engine.NewScriptedController()
	ac.QueueAttackers([]engine.CardID{attacker})
	g.DeclareCombatAttackers(ac)
	bc := engine.NewScriptedController()
	bc.QueueBlocks([]engine.Block{{Blocker: blocker, Attacker: attacker}})
	g.DeclareCombatBlockers(bc)
	g.DealCombatDamage(engine.NewScriptedController())

	if g.Card(blocker).Damage.Marked != 0 {
		t.Errorf("blocker damage marked = %d, want 0 -- the damage is replaced entirely, not reduced", g.Card(blocker).Damage.Marked)
	}
	if got := g.Card(blocker).Counters.Count(engine.P1P1); got != 0 {
		t.Errorf("blocker P1P1 counters = %d, want 0 -- the phantom shape must remove one", got)
	}
}

// TestDamageToPlayerReplacedByPutCounterUsingBareReplaceCount proves the
// bare (operator-less) ReplaceCount$DamageAmount shape -- force_bubble.txt's
// own real "if damage would be dealt to you, put that many depletion
// counters on CARDNAME instead": a 1:1 read of the original amount, distinct
// from resolveDamageReplaceCountAmount's own suffixed Twice/Plus/Minus/
// HalfDown branches.
func TestDamageToPlayerReplacedByPutCounterUsingBareReplaceCount(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	a, b := g.Players()[0], g.Players()[1]
	g.Player(b).Life = 20
	g.SetTurnState(1, a, engine.Main1)
	host := g.NewCard(replacementEnchantmentDefWithSVars(t, "Test Force Bubble",
		"Event$ DamageDone | ValidTarget$ You | ReplaceWith$ Counters | Description$ Put depletion counters instead.",
		map[string]string{
			"Counters": "DB$ PutCounter | Defined$ Self | CounterType$ DEPLETION | CounterNum$ X",
			"X":        "ReplaceCount$DamageAmount",
		}), b, engine.Battlefield)
	attacker := g.NewCard(creatureDefPT(t, "3", "3"), a, engine.Battlefield)

	ac := engine.NewScriptedController()
	ac.QueueAttackers([]engine.CardID{attacker})
	g.DeclareCombatAttackers(ac)
	bc := engine.NewScriptedController()
	bc.QueueBlocks(nil)
	g.DeclareCombatBlockers(bc)
	g.DealCombatDamage(engine.NewScriptedController())

	if g.Player(b).Life != 20 {
		t.Errorf("defender life = %d, want 20 -- the damage is replaced entirely, not just reduced", g.Player(b).Life)
	}
	if got := g.Card(host).Counters.Count(engine.CounterType("DEPLETION")); got != 3 {
		t.Errorf("host DEPLETION counters = %d, want 3 -- the bare ReplaceCount$DamageAmount reads the original 3 damage 1:1", got)
	}
}

// TestDamageToCreatureReplacedByPutCounterOnReplacedTarget proves Defined$
// ReplacedTarget -- soul_scar_mage.txt's own real "put that many -1/-1
// counters on that creature instead" (simplified past its own real
// IsCombat$ False restriction, since this port's only easy damage-dealing
// path in this test file is combat -- the shape under test is Defined$
// ReplacedTarget, not the IsCombat$ gate, which TestDamageToPlayerNot
// ReducedWhenValidSourceDoesNotMatch's own siblings already prove
// elsewhere): the counters land on the DAMAGED creature, not the
// replacement's own host.
func TestDamageToCreatureReplacedByPutCounterOnReplacedTarget(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	a, b := g.Players()[0], g.Players()[1]
	g.SetTurnState(1, a, engine.Main1)
	g.NewCard(replacementEnchantmentDefWithSVars(t, "Test Soul Scar Mage",
		"Event$ DamageDone | ValidSource$ Creature.YouCtrl | ValidTarget$ Creature.OppCtrl | ReplaceWith$ Counters | Description$ Put -1/-1 counters instead.",
		map[string]string{
			"Counters": "DB$ PutCounter | Defined$ ReplacedTarget | CounterType$ M1M1 | CounterNum$ X",
			"X":        "ReplaceCount$DamageAmount",
		}), a, engine.Battlefield)
	attacker := g.NewCard(creatureDefPT(t, "3", "3"), a, engine.Battlefield)
	victim := g.NewCard(creatureDefPT(t, "5", "5"), b, engine.Battlefield)

	ac := engine.NewScriptedController()
	ac.QueueAttackers([]engine.CardID{attacker})
	g.DeclareCombatAttackers(ac)
	bc := engine.NewScriptedController()
	bc.QueueBlocks([]engine.Block{{Blocker: victim, Attacker: attacker}})
	g.DeclareCombatBlockers(bc)
	g.DealCombatDamage(engine.NewScriptedController())

	if g.Card(victim).Damage.Marked != 0 {
		t.Errorf("victim damage marked = %d, want 0 -- the damage is replaced entirely", g.Card(victim).Damage.Marked)
	}
	if got := g.Card(victim).Counters.Count(engine.M1M1); got != 3 {
		t.Errorf("victim M1M1 counters = %d, want 3 -- Defined$ ReplacedTarget must put counters on the damaged creature, not the host", got)
	}
}

// TestDamageToCreatureReplacedByPutCounterOnEquipped proves Defined$
// Equipped -- panther_habit.txt's own real "if equipped creature would be
// dealt damage, prevent that damage and put that many +1/+1 counters on it"
// -- reading Card.AttachedTo() the identical way applyContinuousNames'
// AffectedDefined$ Equipped already does.
func TestDamageToCreatureReplacedByPutCounterOnEquipped(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	a, b := g.Players()[0], g.Players()[1]
	g.SetTurnState(1, a, engine.Main1)
	attacker := g.NewCard(creatureDefPT(t, "3", "3"), a, engine.Battlefield)
	victim := g.NewCard(creatureDefPT(t, "5", "5"), b, engine.Battlefield)
	equipment := g.NewCard(equipmentDefWithReplacementSVars(t, "Test Panther Habit",
		"Event$ DamageDone | ActiveZones$ Battlefield | ValidTarget$ Card.EquippedBy | ReplaceWith$ DBPutCounter | PreventionEffect$ True | AlwaysReplace$ True | Description$ Prevent damage, put +1/+1 counters instead.",
		map[string]string{
			"DBPutCounter": "DB$ PutCounter | Defined$ Equipped | CounterType$ P1P1 | CounterNum$ X",
			"X":            "ReplaceCount$DamageAmount",
		}), b, engine.Battlefield)
	g.Attach(equipment, victim)

	ac := engine.NewScriptedController()
	ac.QueueAttackers([]engine.CardID{attacker})
	g.DeclareCombatAttackers(ac)
	bc := engine.NewScriptedController()
	bc.QueueBlocks([]engine.Block{{Blocker: victim, Attacker: attacker}})
	g.DeclareCombatBlockers(bc)
	g.DealCombatDamage(engine.NewScriptedController())

	if g.Card(victim).Damage.Marked != 0 {
		t.Errorf("victim damage marked = %d, want 0 -- the damage is replaced entirely", g.Card(victim).Damage.Marked)
	}
	if got := g.Card(victim).Counters.Count(engine.P1P1); got != 3 {
		t.Errorf("victim P1P1 counters = %d, want 3 -- Defined$ Equipped must put counters on the equipped creature", got)
	}
}

// TestDamageToCreatureNotReplacedByRemoveCounterWithSubAbility proves a
// chained SubAbility$ refuses outright (GO-7), underdark_beholder.txt's own
// real "remove counters, then sacrifice if none left" shape: the full
// damage applies rather than running the counter removal alone and silently
// dropping the chained sacrifice.
func TestDamageToCreatureNotReplacedByRemoveCounterWithSubAbility(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	a, b := g.Players()[0], g.Players()[1]
	g.SetTurnState(1, a, engine.Main1)
	attacker := g.NewCard(creatureDefPT(t, "3", "3"), a, engine.Battlefield)
	raw := &carddb.Card{Filename: "Test Underdark Beholder"}
	raw.Faces[0].Present = true
	raw.Faces[0].Name = "Test Underdark Beholder"
	raw.Faces[0].Type = cardtype.Parse(attachmentTypeRegistry(t), "Creature Elf")
	raw.Faces[0].Power, raw.Faces[0].Toughness = "5", "5"
	raw.Faces[0].Replacements = []string{
		"Event$ DamageDone | ActiveZones$ Battlefield | ValidTarget$ Card.Self | ReplaceWith$ Counters | Description$ Remove counters instead, then maybe sacrifice.",
	}
	raw.Faces[0].SVars.Set("Counters", "DB$ RemoveCounter | Defined$ ReplacedTarget | CounterType$ EYESTALK | CounterNum$ X | SubAbility$ DBSac")
	raw.Faces[0].SVars.Set("X", "ReplaceCount$DamageAmount")
	raw.Faces[0].SVars.Set("DBSac", "DB$ Sacrifice | SacValid$ Self")
	def, err := compile.Compile(raw)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	blocker := g.NewCard(def, b, engine.Battlefield)
	g.Card(blocker).Counters.Add(engine.CounterType("EYESTALK"), 5)

	ac := engine.NewScriptedController()
	ac.QueueAttackers([]engine.CardID{attacker})
	g.DeclareCombatAttackers(ac)
	bc := engine.NewScriptedController()
	bc.QueueBlocks([]engine.Block{{Blocker: blocker, Attacker: attacker}})
	g.DeclareCombatBlockers(bc)
	g.DealCombatDamage(engine.NewScriptedController())

	if g.Card(blocker).Damage.Marked != 3 {
		t.Errorf("blocker damage marked = %d, want 3 -- a chained SubAbility$ must not dispatch, so the full damage must apply", g.Card(blocker).Damage.Marked)
	}
}

// TestDamageToPlayerNotReplacedByPutCounterNamingCheckDefinedPlayer proves
// jared_carthalion_true_heir.txt's own real R: line, naming
// CheckDefinedPlayer$ You.isMonarch (no monarch mechanic this port tracks),
// is refused outright by damageReplacementMatches' own allow-list before
// applyDamageReplaceCounter is ever reached: the full damage applies.
func TestDamageToPlayerNotReplacedByPutCounterNamingCheckDefinedPlayer(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	a, b := g.Players()[0], g.Players()[1]
	g.SetTurnState(1, a, engine.Main1)
	attacker := g.NewCard(creatureDefPT(t, "3", "3"), a, engine.Battlefield)
	blocker := g.NewCard(replacementCreatureDefPTWithSVar(t, "Test Jared Carthalion", "5", "5",
		"Event$ DamageDone | ActiveZones$ Battlefield | ValidTarget$ Card.Self | CheckDefinedPlayer$ You.isMonarch | ReplaceWith$ Counters | PreventionEffect$ True | AlwaysReplace$ True | Description$ While you're the monarch, prevent damage and put counters instead.",
		"Counters", "DB$ PutCounter | Defined$ ReplacedTarget | CounterType$ P1P1 | CounterNum$ 1"), b, engine.Battlefield)

	ac := engine.NewScriptedController()
	ac.QueueAttackers([]engine.CardID{attacker})
	g.DeclareCombatAttackers(ac)
	bc := engine.NewScriptedController()
	bc.QueueBlocks([]engine.Block{{Blocker: blocker, Attacker: attacker}})
	g.DeclareCombatBlockers(bc)
	g.DealCombatDamage(engine.NewScriptedController())

	if g.Card(blocker).Damage.Marked != 3 {
		t.Errorf("blocker damage marked = %d, want 3 -- CheckDefinedPlayer$ is not resolved, so the whole line must be skipped and full damage must apply", g.Card(blocker).Damage.Marked)
	}
}
