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

	if !g.PlayLand(p, land) {
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

	g.PlayLand(p, land)

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

	g.PlayLand(p, land)

	if g.Card(land).Tapped {
		t.Error("Tapped = true, want false -- a chained SubAbility$ is not resolvable, so the whole line must be skipped")
	}
}

// TestCheckMovedReplacementSkipsConditionalTap proves LandTapped's own real
// shape (135 real lines, "enters tapped unless you control a Mountain or a
// Forest") is skipped too: a ConditionPresent$/ConditionCompare$-qualified
// DB$ Tap is not the unconditional shape this file resolves.
func TestCheckMovedReplacementSkipsConditionalTap(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a")
	p := g.Players()[0]
	g.SetTurnState(1, p, engine.Main1)
	land := g.NewCard(replacementHostDef(t, "Test Checkland", "Land",
		"Event$ Moved | ValidCard$ Card.Self | Destination$ Battlefield | ReplaceWith$ LandTapped",
		"LandTapped", "DB$ Tap | Defined$ Self | ETB$ True | ConditionPresent$ Mountain.YouCtrl | ConditionCompare$ EQ0"), p, engine.Hand)

	g.PlayLand(p, land)

	if g.Card(land).Tapped {
		t.Error("Tapped = true, want false -- a ConditionPresent$-qualified DB$ Tap is not resolvable yet")
	}
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

	if !g.PlayLand(b, land) {
		t.Fatal("PlayLand failed")
	}
	if !g.Card(land).Tapped {
		t.Error("Tapped = false, want true -- Test Tap Enforcer's own ValidCard$ Land.OppCtrl must reach a's opponent's land")
	}
}
