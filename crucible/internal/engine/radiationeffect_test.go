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

// etbRadiationTriggerDefParams builds a *compile.Card whose own "when
// CARDNAME enters" trigger runs DB$ Radiation with the given params string
// appended -- etbDestroyTriggerDefParams' own shape (destroyeffect_test.go).
func etbRadiationTriggerDefParams(t *testing.T, name, extraParams string) *compile.Card {
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
		"Mode$ ChangesZone | Origin$ Any | Destination$ Battlefield | ValidCard$ Card.Self | Execute$ TrigRadiation",
	}
	raw.Faces[0].SVars.Set("TrigRadiation", "DB$ Radiation | "+extraParams)

	c, err := compile.Compile(raw)
	if err != nil {
		t.Fatalf("compile %q: %v", name, err)
	}
	return c
}

// castETBRadiation casts def for p on controller c and resolves the stack
// -- castETBDestroy's own shape (destroyeffect_test.go).
func castETBRadiation(t *testing.T, g *engine.Game, p engine.PlayerID, def *compile.Card, c *engine.ScriptedController) (engine.CardID, error) {
	t.Helper()
	g.Player(p).ManaPool.Add(mana.Green, 1)
	creature := g.NewCard(def, p, engine.Hand)
	if !g.CastSpell(p, creature, c) {
		t.Fatal("CastSpell failed casting a creature with exactly enough mana")
	}
	return creature, g.ResolveStack(engine.NewRegistry(), c)
}

// TestRadiationEffectAddsCountersToDefinedPlayer proves the corpus's own
// dominant real shape: Defined$ adds Num$ radiation counters.
func TestRadiationEffectAddsCountersToDefinedPlayer(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p := g.Players()[0]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life, g.Player(g.Players()[1]).Life = 20, 20

	c := engine.NewScriptedController()
	_, err := castETBRadiation(t, g, p, etbRadiationTriggerDefParams(t, "Test Radiation Add", "Num$ 3 | Defined$ You"), c)
	if err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if n := g.Player(p).Counters.Count(engine.Radiation); n != 3 {
		t.Errorf("p radiation = %d, want 3", n)
	}
}

// TestRadiationEffectValidTgtsScopesToChosenPlayer proves an ability naming
// ValidTgts$ reads its own chosen player target directly
// (targetedOrDefinedPlayers, defined.go).
func TestRadiationEffectValidTgtsScopesToChosenPlayer(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p, other := g.Players()[0], g.Players()[1]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life, g.Player(other).Life = 20, 20

	c := engine.NewScriptedController()
	c.QueueTargets([]engine.EntityID{engine.PlayerEntity(other)})
	_, err := castETBRadiation(t, g, p, etbRadiationTriggerDefParams(t, "Test Radiation Tgt", "Num$ 2 | ValidTgts$ Player"), c)
	if err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if n := g.Player(other).Counters.Count(engine.Radiation); n != 2 {
		t.Errorf("other radiation = %d, want 2", n)
	}
	if n := g.Player(p).Counters.Count(engine.Radiation); n != 0 {
		t.Errorf("p radiation = %d, want 0", n)
	}
}

// TestRadiationEffectRejectsUnresolvedParam proves radiationUnresolvedParams'
// own fail-loud contract (PORT-8/GO-7).
func TestRadiationEffectRejectsUnresolvedParam(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p := g.Players()[0]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life, g.Player(g.Players()[1]).Life = 20, 20

	c := engine.NewScriptedController()
	_, err := castETBRadiation(t, g, p, etbRadiationTriggerDefParams(t, "Test Radiation TriggeredCard", "Num$ 1 | Defined$ You | TriggeredCard$ True"), c)
	if err == nil {
		t.Fatal("ResolveStack succeeded, want an error for unresolved TriggeredCard$")
	}
}
