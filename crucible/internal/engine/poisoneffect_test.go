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

// etbPoisonTriggerDefParams builds a *compile.Card whose own "when CARDNAME
// enters" trigger runs DB$ Poison with the given params string appended --
// etbDestroyTriggerDefParams' own shape (destroyeffect_test.go).
func etbPoisonTriggerDefParams(t *testing.T, name, extraParams string) *compile.Card {
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
		"Mode$ ChangesZone | Origin$ Any | Destination$ Battlefield | ValidCard$ Card.Self | Execute$ TrigPoison",
	}
	raw.Faces[0].SVars.Set("TrigPoison", "DB$ Poison | "+extraParams)

	c, err := compile.Compile(raw)
	if err != nil {
		t.Fatalf("compile %q: %v", name, err)
	}
	return c
}

// castETBPoison casts def for p on controller c and resolves the stack --
// castETBDestroy's own shape (destroyeffect_test.go).
func castETBPoison(t *testing.T, g *engine.Game, p engine.PlayerID, def *compile.Card, c *engine.ScriptedController) (engine.CardID, error) {
	t.Helper()
	g.Player(p).ManaPool.Add(mana.Green, 1)
	creature := g.NewCard(def, p, engine.Hand)
	if !g.CastSpell(p, creature, c) {
		t.Fatal("CastSpell failed casting a creature with exactly enough mana")
	}
	return creature, g.ResolveStack(engine.NewRegistry(), c)
}

// TestPoisonEffectAddsCountersToDefinedPlayer proves the corpus's own
// dominant real shape: Defined$ adds Num$ poison counters.
func TestPoisonEffectAddsCountersToDefinedPlayer(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p, other := g.Players()[0], g.Players()[1]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life, g.Player(other).Life = 20, 20

	c := engine.NewScriptedController()
	_, err := castETBPoison(t, g, p, etbPoisonTriggerDefParams(t, "Test Poison Add", "Num$ 3 | Defined$ Opponent"), c)
	if err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if n := g.Player(other).Counters.Count(engine.Poison); n != 3 {
		t.Errorf("other poison = %d, want 3", n)
	}
}

// TestPoisonEffectNegativeNumRemoves proves a negative Num$ removes poison
// counters instead of adding them.
func TestPoisonEffectNegativeNumRemoves(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p := g.Players()[0]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life, g.Player(g.Players()[1]).Life = 20, 20
	g.Player(p).Counters.Add(engine.Poison, 4)

	c := engine.NewScriptedController()
	_, err := castETBPoison(t, g, p, etbPoisonTriggerDefParams(t, "Test Poison Remove", "Num$ -1 | Defined$ You"), c)
	if err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if n := g.Player(p).Counters.Count(engine.Poison); n != 3 {
		t.Errorf("p poison = %d, want 3", n)
	}
}

// TestPoisonEffectValidTgtsScopesToChosenPlayer proves an ability naming
// ValidTgts$ reads its own chosen player target directly
// (targetedOrDefinedPlayers, defined.go).
func TestPoisonEffectValidTgtsScopesToChosenPlayer(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p, other := g.Players()[0], g.Players()[1]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life, g.Player(other).Life = 20, 20

	c := engine.NewScriptedController()
	c.QueueTargets([]engine.EntityID{engine.PlayerEntity(other)})
	_, err := castETBPoison(t, g, p, etbPoisonTriggerDefParams(t, "Test Poison Tgt", "Num$ 2 | ValidTgts$ Player"), c)
	if err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if n := g.Player(other).Counters.Count(engine.Poison); n != 2 {
		t.Errorf("other poison = %d, want 2", n)
	}
	if n := g.Player(p).Counters.Count(engine.Poison); n != 0 {
		t.Errorf("p poison = %d, want 0", n)
	}
}

// TestPoisonEffectRejectsUnresolvedParam proves poisonUnresolvedParams' own
// fail-loud contract (PORT-8/GO-7).
func TestPoisonEffectRejectsUnresolvedParam(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p := g.Players()[0]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life, g.Player(g.Players()[1]).Life = 20, 20

	c := engine.NewScriptedController()
	_, err := castETBPoison(t, g, p, etbPoisonTriggerDefParams(t, "Test Poison Ultimate", "Num$ 1 | Defined$ You | Ultimate$ True"), c)
	if err == nil {
		t.Fatal("ResolveStack succeeded, want an error for unresolved Ultimate$")
	}
}
