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

// etbShuffleTriggerDefParams builds a *compile.Card whose own "when
// CARDNAME enters" trigger runs DB$ Shuffle with the given params string
// appended -- etbDestroyTriggerDefParams' own shape (destroyeffect_test.go).
func etbShuffleTriggerDefParams(t *testing.T, name, extraParams string) *compile.Card {
	t.Helper()

	reg, err := cardtype.LoadRegistry(strings.NewReader("[CreatureTypes]\nElf\n"))
	if err != nil {
		t.Fatalf("LoadRegistry: %v", err)
	}
	raw := &carddb.Card{Filename: name}
	raw.Faces[0].Present = true
	raw.Faces[0].Name = name
	raw.Faces[0].Type = cardtype.Parse(reg, "Creature Elf")
	raw.Faces[0].Power, raw.Faces[0].Toughness = "1", "1"
	raw.Faces[0].ManaCost = mana.MustParse("G")
	raw.Faces[0].Triggers = []string{
		"Mode$ ChangesZone | Origin$ Any | Destination$ Battlefield | ValidCard$ Card.Self | Execute$ TrigShuffle",
	}
	raw.Faces[0].SVars.Set("TrigShuffle", "DB$ Shuffle | "+extraParams)

	c, err := compile.Compile(raw)
	if err != nil {
		t.Fatalf("compile %q: %v", name, err)
	}
	return c
}

// castETBShuffle casts def for p on controller c and resolves the stack --
// castETBDestroy's own shape (destroyeffect_test.go).
func castETBShuffle(t *testing.T, g *engine.Game, p engine.PlayerID, def *compile.Card, c *engine.ScriptedController) (engine.CardID, error) {
	t.Helper()
	g.Player(p).ManaPool.Add(mana.Green, 1)
	creature := g.NewCard(def, p, engine.Hand)
	if !g.CastSpell(p, creature, c) {
		t.Fatal("CastSpell failed casting a creature with exactly enough mana")
	}
	return creature, g.ResolveStack(engine.NewRegistry(), c)
}

// cardSet turns a []CardID into a comparable set, order-independent -- this
// file's own tests prove the RIGHT library got shuffled, not what order
// Game.Shuffle's own javarand stream happens to produce for a fixed seed
// (game.go already proves the algorithm itself, pkg/javarand's P0 gate).
func cardSet(ids []engine.CardID) map[engine.CardID]bool {
	m := make(map[engine.CardID]bool, len(ids))
	for _, id := range ids {
		m[id] = true
	}
	return m
}

func sameCardSet(a, b []engine.CardID) bool {
	if len(a) != len(b) {
		return false
	}
	sa := cardSet(a)
	for _, id := range b {
		if !sa[id] {
			return false
		}
	}
	return true
}

// TestShuffleEffectDefinedYouShufflesOwnLibrary proves the corpus's own
// dominant real shape: Defined$ You shuffles the activating player's own
// library, leaving the same cards present (Game.Shuffle reorders in place,
// game.go's own doc comment).
func TestShuffleEffectDefinedYouShufflesOwnLibrary(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p := g.Players()[0]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life, g.Player(g.Players()[1]).Life = 20, 20
	before := libraryCards(t, g, p, 6)

	c := engine.NewScriptedController()
	_, err := castETBShuffle(t, g, p, etbShuffleTriggerDefParams(t, "Test Shuffle You", "Defined$ You"), c)
	if err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	after := g.Zone(engine.Library, p).Cards()
	if !sameCardSet(before, after) {
		t.Errorf("library cards after shuffle = %v, want the same set as before = %v", after, before)
	}
}

// TestShuffleEffectValidTgtsShufflesChosenPlayersLibraryOnly proves an
// ability naming ValidTgts$ reads its own chosen player target directly
// (targetedOrDefinedPlayers, defined.go) and leaves every OTHER player's
// library alone.
func TestShuffleEffectValidTgtsShufflesChosenPlayersLibraryOnly(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p, other := g.Players()[0], g.Players()[1]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life, g.Player(other).Life = 20, 20
	mine := libraryCards(t, g, p, 3)
	theirs := libraryCards(t, g, other, 3)

	c := engine.NewScriptedController()
	c.QueueTargets([]engine.EntityID{engine.PlayerEntity(other)})
	_, err := castETBShuffle(t, g, p, etbShuffleTriggerDefParams(t, "Test Shuffle Tgt", "ValidTgts$ Player"), c)
	if err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if got := g.Zone(engine.Library, other).Cards(); !sameCardSet(theirs, got) {
		t.Errorf("other library after shuffle = %v, want the same set as before = %v", got, theirs)
	}
	mineAfter := g.Zone(engine.Library, p).Cards()
	if len(mineAfter) != len(mine) {
		t.Errorf("p library size = %d, want %d unchanged -- only the targeted player shuffles", len(mineAfter), len(mine))
	}
	for i, id := range mine {
		if mineAfter[i] != id {
			t.Errorf("p library order changed at %d = %v, want %v unchanged -- only the targeted player shuffles", i, mineAfter[i], id)
			break
		}
	}
}

// TestShuffleEffectRejectsUnresolvedParam proves shuffleUnresolvedParams'
// own fail-loud contract (PORT-8/GO-7).
func TestShuffleEffectRejectsUnresolvedParam(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p := g.Players()[0]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life, g.Player(g.Players()[1]).Life = 20, 20
	libraryCards(t, g, p, 2)

	c := engine.NewScriptedController()
	_, err := castETBShuffle(t, g, p, etbShuffleTriggerDefParams(t, "Test Shuffle Optional", "Defined$ You | Optional$ True"), c)
	if err == nil {
		t.Fatal("ResolveStack succeeded, want an error for unresolved Optional$")
	}
}
