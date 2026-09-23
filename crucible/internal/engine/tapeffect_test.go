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

// etbTapTriggerDefParams builds a *compile.Card whose own "when CARDNAME
// enters" trigger runs DB$ Tap with the given params string appended --
// etbDestroyTriggerDefParams' own shape (destroyeffect_test.go).
func etbTapTriggerDefParams(t *testing.T, name, extraParams string) *compile.Card {
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
		"Mode$ ChangesZone | Origin$ Any | Destination$ Battlefield | ValidCard$ Card.Self | Execute$ TrigTap",
	}
	raw.Faces[0].SVars.Set("TrigTap", "DB$ Tap | "+extraParams)

	c, err := compile.Compile(raw)
	if err != nil {
		t.Fatalf("compile %q: %v", name, err)
	}
	return c
}

// castETBTap casts def for p on controller c and resolves the stack --
// castETBDestroy's own shape (destroyeffect_test.go).
func castETBTap(t *testing.T, g *engine.Game, p engine.PlayerID, def *compile.Card, c *engine.ScriptedController) (engine.CardID, error) {
	t.Helper()
	g.Player(p).ManaPool.Add(mana.Green, 1)
	creature := g.NewCard(def, p, engine.Hand)
	if !g.CastSpell(p, creature, c) {
		t.Fatal("CastSpell failed casting a creature with exactly enough mana")
	}
	return creature, g.ResolveStack(engine.NewRegistry(), c)
}

// TestTapEffectValidTgtsTapsChosenTarget proves the corpus's own dominant
// real shape past ETB$ (royal_assassin.txt's own "ValidTgts$ Creature.
// tapped" shape mirrored here for an untapped target instead): an ability
// naming ValidTgts$ reads its own chosen target directly (a.Targets,
// targetedOrDefinedCards, defined.go), and taps it, firing checkTapsTriggers
// (trigger.go).
func TestTapEffectValidTgtsTapsChosenTarget(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p := g.Players()[0]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life, g.Player(g.Players()[1]).Life = 20, 20
	target := g.NewCard(creatureDefPT(t, "2", "2"), p, engine.Battlefield)

	c := engine.NewScriptedController()
	c.QueueTargets([]engine.EntityID{engine.CardEntity(target)})

	_, err := castETBTap(t, g, p, etbTapTriggerDefParams(t, "Test Tap Tgt", "ValidTgts$ Creature"), c)
	if err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if !g.Card(target).Tapped {
		t.Error("target.Tapped = false, want true")
	}
}

// TestTapEffectDefinedSelfTapsHost proves an ability naming no ValidTgts$ at
// all falls back to Defined$, defaulting to "Self" the same way Java's own
// getParamOrDefault(definedParam, "Self") does.
func TestTapEffectDefinedSelfTapsHost(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p := g.Players()[0]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life, g.Player(g.Players()[1]).Life = 20, 20

	c := engine.NewScriptedController()
	creature, err := castETBTap(t, g, p, etbTapTriggerDefParams(t, "Test Tap Self", "Defined$ Self"), c)
	if err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if !g.Card(creature).Tapped {
		t.Error("host.Tapped = false, want true -- Defined$ Self must tap the host itself")
	}
}

// TestTapEffectAlreadyTappedTargetStaysUntouched proves Card.tap()'s own
// early "if (tapped) return false" -- no double-fire of checkTapsTriggers on
// a target already tapped before this effect runs.
func TestTapEffectAlreadyTappedTargetStaysUntouched(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p := g.Players()[0]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life, g.Player(g.Players()[1]).Life = 20, 20
	target := g.NewCard(creatureDefPT(t, "2", "2"), p, engine.Battlefield)
	g.Card(target).Tapped = true

	c := engine.NewScriptedController()
	c.QueueTargets([]engine.EntityID{engine.CardEntity(target)})

	_, err := castETBTap(t, g, p, etbTapTriggerDefParams(t, "Test Tap Already", "ValidTgts$ Creature"), c)
	if err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if !g.Card(target).Tapped {
		t.Error("target.Tapped = false, want true (still tapped)")
	}
}

// TestTapEffectRememberTappedWritesMemory proves RememberTapped$ writes the
// tapped card onto the ability's own host card's Memory --
// TestSacrificeEffectRememberSacrificedWritesMemory's own shape
// (sacrificeeffect_test.go).
func TestTapEffectRememberTappedWritesMemory(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p := g.Players()[0]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life, g.Player(g.Players()[1]).Life = 20, 20
	target := g.NewCard(creatureDefPT(t, "2", "2"), p, engine.Battlefield)

	c := engine.NewScriptedController()
	c.QueueTargets([]engine.EntityID{engine.CardEntity(target)})

	creature, err := castETBTap(t, g, p, etbTapTriggerDefParams(t, "Test Tap Remember", "ValidTgts$ Creature | RememberTapped$ True"), c)
	if err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	remembered := g.Card(creature).Memory.Remembered()
	if len(remembered) != 1 || remembered[0] != engine.CardEntity(target) {
		t.Errorf("host's Remembered() = %v, want [%v]", remembered, engine.CardEntity(target))
	}
}

// TestTapEffectRejectsUnresolvedParam proves tapUnresolvedParams' own
// fail-loud contract (PORT-8/GO-7).
func TestTapEffectRejectsUnresolvedParam(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p := g.Players()[0]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life, g.Player(g.Players()[1]).Life = 20, 20
	target := g.NewCard(creatureDefPT(t, "2", "2"), p, engine.Battlefield)

	c := engine.NewScriptedController()
	c.QueueTargets([]engine.EntityID{engine.CardEntity(target)})

	_, err := castETBTap(t, g, p, etbTapTriggerDefParams(t, "Test Tap Tapper", "ValidTgts$ Creature | Tapper$ You"), c)
	if err == nil {
		t.Fatal("ResolveStack succeeded, want an error for unresolved Tapper$")
	}
}
