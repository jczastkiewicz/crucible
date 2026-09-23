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

// etbMillTriggerDefParams builds a *compile.Card whose own "when CARDNAME
// enters" trigger runs DB$ Mill with the given params string appended --
// etbDestroyTriggerDefParams' own shape (destroyeffect_test.go).
func etbMillTriggerDefParams(t *testing.T, name, extraParams string) *compile.Card {
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
		"Mode$ ChangesZone | Origin$ Any | Destination$ Battlefield | ValidCard$ Card.Self | Execute$ TrigMill",
	}
	raw.Faces[0].SVars.Set("TrigMill", "DB$ Mill | "+extraParams)

	c, err := compile.Compile(raw)
	if err != nil {
		t.Fatalf("compile %q: %v", name, err)
	}
	return c
}

// castETBMill casts def for p on controller c and resolves the stack --
// castETBDestroy's own shape (destroyeffect_test.go).
func castETBMill(t *testing.T, g *engine.Game, p engine.PlayerID, def *compile.Card, c *engine.ScriptedController) (engine.CardID, error) {
	t.Helper()
	g.Player(p).ManaPool.Add(mana.Green, 1)
	creature := g.NewCard(def, p, engine.Hand)
	if !g.CastSpell(p, creature, c) {
		t.Fatal("CastSpell failed casting a creature with exactly enough mana")
	}
	return creature, g.ResolveStack(engine.NewRegistry(), c)
}

// libraryCards puts n vanilla cards into p's library, top to bottom in the
// order given, and returns their CardIDs in that same top-to-bottom order.
func libraryCards(t *testing.T, g *engine.Game, p engine.PlayerID, n int) []engine.CardID {
	t.Helper()
	ids := make([]engine.CardID, n)
	for i := 0; i < n; i++ {
		ids[i] = g.NewCard(creatureDefPT(t, "1", "1"), p, engine.Library)
	}
	return ids
}

// TestMillEffectDefinedYouMillsTopOfOwnLibrary proves Mill's own dominant
// real shape (317 of 507 real lines): Defined$ resolves the miller, and the
// top NumCards$ of their library moves to the graveyard, top card first --
// GameAction.mill's own Iterables.limit(milledView, n) reading library order
// starting from the top, ScryEffect's own identical topN reasoning
// (scryeffect.go).
func TestMillEffectDefinedYouMillsTopOfOwnLibrary(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p := g.Players()[0]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life, g.Player(g.Players()[1]).Life = 20, 20
	lib := libraryCards(t, g, p, 5)

	c := engine.NewScriptedController()
	_, err := castETBMill(t, g, p, etbMillTriggerDefParams(t, "Test Mill You", "NumCards$ 2 | Defined$ You"), c)
	if err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	for i, id := range lib[:2] {
		if g.Card(id).Zone != engine.Graveyard {
			t.Errorf("lib[%d] zone = %v, want Graveyard (milled)", i, g.Card(id).Zone)
		}
	}
	for i, id := range lib[2:] {
		if g.Card(id).Zone != engine.Library {
			t.Errorf("lib[%d] zone = %v, want Library (not milled)", 2+i, g.Card(id).Zone)
		}
	}
}

// TestMillEffectValidTgtsMillsChosenPlayer proves an ability naming
// ValidTgts$ reads its own chosen player target directly
// (targetedOrDefinedPlayers, defined.go) rather than through Defined$.
func TestMillEffectValidTgtsMillsChosenPlayer(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p, other := g.Players()[0], g.Players()[1]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life, g.Player(other).Life = 20, 20
	otherLib := libraryCards(t, g, other, 3)

	c := engine.NewScriptedController()
	c.QueueTargets([]engine.EntityID{engine.PlayerEntity(other)})
	_, err := castETBMill(t, g, p, etbMillTriggerDefParams(t, "Test Mill Tgt", "NumCards$ 1 | ValidTgts$ Player"), c)
	if err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if g.Card(otherLib[0]).Zone != engine.Graveyard {
		t.Error("other's top library card was not milled")
	}
}

// TestMillEffectClampsToLibrarySize proves GameAction.mill's own
// Iterables.limit behavior: milling more cards than the library holds mills
// however many actually exist, rather than erroring or milling nothing.
func TestMillEffectClampsToLibrarySize(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p := g.Players()[0]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life, g.Player(g.Players()[1]).Life = 20, 20
	lib := libraryCards(t, g, p, 2)

	c := engine.NewScriptedController()
	_, err := castETBMill(t, g, p, etbMillTriggerDefParams(t, "Test Mill Clamp", "NumCards$ 10 | Defined$ You"), c)
	if err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	for i, id := range lib {
		if g.Card(id).Zone != engine.Graveyard {
			t.Errorf("lib[%d] zone = %v, want Graveyard", i, g.Card(id).Zone)
		}
	}
}

// TestMillEffectRememberMilledWritesMemory proves RememberMilled$ writes
// every milled card onto the ability's own host card's Memory --
// TestSacrificeEffectRememberSacrificedWritesMemory's own shape
// (sacrificeeffect_test.go), extended to more than one remembered card.
func TestMillEffectRememberMilledWritesMemory(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p := g.Players()[0]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life, g.Player(g.Players()[1]).Life = 20, 20
	lib := libraryCards(t, g, p, 2)

	c := engine.NewScriptedController()
	host, err := castETBMill(t, g, p, etbMillTriggerDefParams(t, "Test Mill Remember", "NumCards$ 2 | Defined$ You | RememberMilled$ True"), c)
	if err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	remembered := g.Card(host).Memory.Remembered()
	if len(remembered) != 2 || remembered[0] != engine.CardEntity(lib[0]) || remembered[1] != engine.CardEntity(lib[1]) {
		t.Errorf("host's Remembered() = %v, want [%v %v]", remembered, engine.CardEntity(lib[0]), engine.CardEntity(lib[1]))
	}
}

// TestMillEffectZeroNumCardsIsNoOp proves CR 701.13b's own "a player
// instructed to mill 0 cards does not mill" -- MillEffect.resolve's own
// leading `if (numCards <= 0) return`.
func TestMillEffectZeroNumCardsIsNoOp(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p := g.Players()[0]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life, g.Player(g.Players()[1]).Life = 20, 20
	lib := libraryCards(t, g, p, 2)

	c := engine.NewScriptedController()
	_, err := castETBMill(t, g, p, etbMillTriggerDefParams(t, "Test Mill Zero", "NumCards$ 0 | Defined$ You"), c)
	if err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	for i, id := range lib {
		if g.Card(id).Zone != engine.Library {
			t.Errorf("lib[%d] zone = %v, want Library (NumCards$ 0 mills nothing)", i, g.Card(id).Zone)
		}
	}
}

// TestMillEffectRejectsUnresolvedParam proves millUnresolvedParams' own
// fail-loud contract (PORT-8/GO-7).
func TestMillEffectRejectsUnresolvedParam(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p := g.Players()[0]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life, g.Player(g.Players()[1]).Life = 20, 20
	libraryCards(t, g, p, 2)

	c := engine.NewScriptedController()
	_, err := castETBMill(t, g, p, etbMillTriggerDefParams(t, "Test Mill Ultimate", "NumCards$ 1 | Defined$ You | Ultimate$ True"), c)
	if err == nil {
		t.Fatal("ResolveStack succeeded, want an error for unresolved Ultimate$")
	}
}
