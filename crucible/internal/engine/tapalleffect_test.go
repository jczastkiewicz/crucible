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

// etbTapAllTriggerDefParams builds a *compile.Card whose own "when CARDNAME
// enters" trigger runs DB$ TapAll with the given params string appended --
// etbDestroyTriggerDefParams' own shape (destroyeffect_test.go).
func etbTapAllTriggerDefParams(t *testing.T, name, extraParams string) *compile.Card {
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
		"Mode$ ChangesZone | Origin$ Any | Destination$ Battlefield | ValidCard$ Card.Self | Execute$ TrigTapAll",
	}
	raw.Faces[0].SVars.Set("TrigTapAll", "DB$ TapAll | "+extraParams)

	c, err := compile.Compile(raw)
	if err != nil {
		t.Fatalf("compile %q: %v", name, err)
	}
	return c
}

// castETBTapAll casts def for p on controller c and resolves the stack --
// castETBDestroy's own shape (destroyeffect_test.go).
func castETBTapAll(t *testing.T, g *engine.Game, p engine.PlayerID, def *compile.Card, c *engine.ScriptedController) (engine.CardID, error) {
	t.Helper()
	g.Player(p).ManaPool.Add(mana.Green, 1)
	creature := g.NewCard(def, p, engine.Hand)
	if !g.CastSpell(p, creature, c) {
		t.Fatal("CastSpell failed casting a creature with exactly enough mana")
	}
	return creature, g.ResolveStack(engine.NewRegistry(), c)
}

// TestTapAllEffectSweepsEveryPlayersBattlefield proves the corpus's own
// dominant real shape: neither ValidTgts$ nor Defined$ present sweeps every
// player's battlefield, not just the activator's own.
func TestTapAllEffectSweepsEveryPlayersBattlefield(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p, other := g.Players()[0], g.Players()[1]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life, g.Player(other).Life = 20, 20
	mine := g.NewCard(creatureDefPT(t, "2", "2"), p, engine.Battlefield)
	theirs := g.NewCard(creatureDefPT(t, "2", "2"), other, engine.Battlefield)

	c := engine.NewScriptedController()
	_, err := castETBTapAll(t, g, p, etbTapAllTriggerDefParams(t, "Test TapAll Sweep", "ValidCards$ Creature"), c)
	if err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if !g.Card(mine).Tapped {
		t.Error("mine tapped = false, want true")
	}
	if !g.Card(theirs).Tapped {
		t.Error("theirs tapped = false, want true")
	}
}

// TestTapAllEffectValidTgtsScopesToTargetedPlayer proves an ability naming
// ValidTgts$ reads its own chosen player target (targetedOrDefinedPlayers,
// defined.go) and leaves every OTHER player's battlefield alone.
func TestTapAllEffectValidTgtsScopesToTargetedPlayer(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p, other := g.Players()[0], g.Players()[1]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life, g.Player(other).Life = 20, 20
	mine := g.NewCard(creatureDefPT(t, "2", "2"), p, engine.Battlefield)
	theirs := g.NewCard(creatureDefPT(t, "2", "2"), other, engine.Battlefield)

	c := engine.NewScriptedController()
	c.QueueTargets([]engine.EntityID{engine.PlayerEntity(other)})
	_, err := castETBTapAll(t, g, p, etbTapAllTriggerDefParams(t, "Test TapAll Tgt", "ValidCards$ Creature | ValidTgts$ Player"), c)
	if err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if !g.Card(theirs).Tapped {
		t.Error("theirs tapped = false, want true")
	}
	if g.Card(mine).Tapped {
		t.Error("mine tapped = true, want false -- only the targeted player's battlefield sweeps")
	}
}

// TestTapAllEffectRememberTapped proves RememberTapped$ remembers every
// card this resolution actually taps onto the host's own Memory.
func TestTapAllEffectRememberTapped(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p := g.Players()[0]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life, g.Player(g.Players()[1]).Life = 20, 20
	mine := g.NewCard(creatureDefPT(t, "2", "2"), p, engine.Battlefield)

	c := engine.NewScriptedController()
	host, err := castETBTapAll(t, g, p, etbTapAllTriggerDefParams(t, "Test TapAll Remember", "ValidCards$ Creature.StrictlyOther | RememberTapped$ True"), c)
	if err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	remembered := g.Card(host).Memory.Remembered()
	if len(remembered) != 1 || remembered[0] != engine.CardEntity(mine) {
		t.Errorf("host remembered = %v, want [%v]", remembered, engine.CardEntity(mine))
	}
}

// TestTapAllEffectRejectsUnresolvedParam proves tapAllUnresolvedParams' own
// fail-loud contract (PORT-8/GO-7).
func TestTapAllEffectRejectsUnresolvedParam(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p := g.Players()[0]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life, g.Player(g.Players()[1]).Life = 20, 20

	c := engine.NewScriptedController()
	_, err := castETBTapAll(t, g, p, etbTapAllTriggerDefParams(t, "Test TapAll Planeswalker", "ValidCards$ Creature | Planeswalker$ True"), c)
	if err == nil {
		t.Fatal("ResolveStack succeeded, want an error for unresolved Planeswalker$")
	}
}
