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

// etbUntapTriggerDefParams builds a *compile.Card whose own "when CARDNAME
// enters" trigger runs DB$ Untap with the given params string appended --
// etbTapTriggerDefParams' own shape (tapeffect_test.go).
func etbUntapTriggerDefParams(t *testing.T, name, extraParams string) *compile.Card {
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
		"Mode$ ChangesZone | Origin$ Any | Destination$ Battlefield | ValidCard$ Card.Self | Execute$ TrigUntap",
	}
	raw.Faces[0].SVars.Set("TrigUntap", "DB$ Untap | "+extraParams)

	c, err := compile.Compile(raw)
	if err != nil {
		t.Fatalf("compile %q: %v", name, err)
	}
	return c
}

// castETBUntap casts def for p on controller c and resolves the stack --
// castETBTap's own shape (tapeffect_test.go).
func castETBUntap(t *testing.T, g *engine.Game, p engine.PlayerID, def *compile.Card, c *engine.ScriptedController) (engine.CardID, error) {
	t.Helper()
	g.Player(p).ManaPool.Add(mana.Green, 1)
	creature := g.NewCard(def, p, engine.Hand)
	if !g.CastSpell(p, creature, c) {
		t.Fatal("CastSpell failed casting a creature with exactly enough mana")
	}
	return creature, g.ResolveStack(engine.NewRegistry(), c)
}

// TestUntapEffectValidTgtsUntapsChosenTarget proves an ability naming
// ValidTgts$ reads its own chosen target directly (a.Targets,
// targetedOrDefinedCards, defined.go), untaps it, and fires
// checkUntapsTriggers (trigger.go, Mode$ Untaps).
func TestUntapEffectValidTgtsUntapsChosenTarget(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p := g.Players()[0]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life, g.Player(g.Players()[1]).Life = 20, 20
	target := g.NewCard(creatureDefPT(t, "2", "2"), p, engine.Battlefield)
	g.Card(target).Tapped = true

	c := engine.NewScriptedController()
	c.QueueTargets([]engine.EntityID{engine.CardEntity(target)})

	_, err := castETBUntap(t, g, p, etbUntapTriggerDefParams(t, "Test Untap Tgt", "ValidTgts$ Creature"), c)
	if err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if g.Card(target).Tapped {
		t.Error("target.Tapped = true, want false")
	}
}

// TestUntapEffectDefinedSelfUntapsHost proves an ability naming no
// ValidTgts$ at all falls back to Defined$ -- Untap's own dominant real
// corpus shape (228 of 431 non-UntapUpTo/UntapExactly lines), unlike
// Destroy's/Tap's own ValidTgts$-dominant shape (the file doc comment,
// untapeffect.go).
func TestUntapEffectDefinedSelfUntapsHost(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p := g.Players()[0]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life, g.Player(g.Players()[1]).Life = 20, 20

	c := engine.NewScriptedController()
	creature := g.NewCard(etbUntapTriggerDefParams(t, "Test Untap Self", "Defined$ Self"), p, engine.Hand)
	g.Card(creature).Tapped = true
	g.Player(p).ManaPool.Add(mana.Green, 1)
	if !g.CastSpell(p, creature, c) {
		t.Fatal("CastSpell failed casting a creature with exactly enough mana")
	}
	if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if g.Card(creature).Tapped {
		t.Error("host.Tapped = true, want false -- Defined$ Self must untap the host itself")
	}
}

// TestUntapEffectAlreadyUntappedTargetStaysUntouched proves the symmetric
// early-return TestTapEffectAlreadyTappedTargetStaysUntouched (tapeffect_
// test.go) proves for Tap: no double-fire of checkUntapsTriggers on a
// target already untapped before this effect runs.
func TestUntapEffectAlreadyUntappedTargetStaysUntouched(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p := g.Players()[0]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life, g.Player(g.Players()[1]).Life = 20, 20
	target := g.NewCard(creatureDefPT(t, "2", "2"), p, engine.Battlefield)

	c := engine.NewScriptedController()
	c.QueueTargets([]engine.EntityID{engine.CardEntity(target)})

	_, err := castETBUntap(t, g, p, etbUntapTriggerDefParams(t, "Test Untap Already", "ValidTgts$ Creature"), c)
	if err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if g.Card(target).Tapped {
		t.Error("target.Tapped = true, want false (still untapped)")
	}
}

// TestUntapEffectRejectsUnresolvedParam proves untapUnresolvedParams' own
// fail-loud contract (PORT-8/GO-7).
func TestUntapEffectRejectsUnresolvedParam(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p := g.Players()[0]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life, g.Player(g.Players()[1]).Life = 20, 20
	target := g.NewCard(creatureDefPT(t, "2", "2"), p, engine.Battlefield)
	g.Card(target).Tapped = true

	c := engine.NewScriptedController()
	c.QueueTargets([]engine.EntityID{engine.CardEntity(target)})

	_, err := castETBUntap(t, g, p, etbUntapTriggerDefParams(t, "Test Untap ETB", "ValidTgts$ Creature | ETB$ True"), c)
	if err == nil {
		t.Fatal("ResolveStack succeeded, want an error for unresolved ETB$")
	}
}
