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

// etbSetLifeTriggerDefParams builds a *compile.Card whose own "when
// CARDNAME enters" trigger runs DB$ SetLife with the given params string
// appended -- etbDestroyTriggerDefParams' own shape (destroyeffect_test.go).
func etbSetLifeTriggerDefParams(t *testing.T, name, extraParams string) *compile.Card {
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
		"Mode$ ChangesZone | Origin$ Any | Destination$ Battlefield | ValidCard$ Card.Self | Execute$ TrigSetLife",
	}
	raw.Faces[0].SVars.Set("TrigSetLife", "DB$ SetLife | "+extraParams)

	c, err := compile.Compile(raw)
	if err != nil {
		t.Fatalf("compile %q: %v", name, err)
	}
	return c
}

// castETBSetLife casts def for p on controller c and resolves the stack --
// castETBDestroy's own shape (destroyeffect_test.go).
func castETBSetLife(t *testing.T, g *engine.Game, p engine.PlayerID, def *compile.Card, c *engine.ScriptedController) (engine.CardID, error) {
	t.Helper()
	g.Player(p).ManaPool.Add(mana.Green, 1)
	creature := g.NewCard(def, p, engine.Hand)
	if !g.CastSpell(p, creature, c) {
		t.Fatal("CastSpell failed casting a creature with exactly enough mana")
	}
	return creature, g.ResolveStack(engine.NewRegistry(), c)
}

// TestSetLifeEffectAboveCurrentIsAGain proves Player.setLife's own CR 119.5
// dispatch: a life total set ABOVE its current value routes through the
// same gain machinery GainLife uses, so LifeGainedTimesThisTurn -- the
// "first gain this turn" bookkeeping checkLifeGainedTriggers reads -- moves
// too, not just the raw total.
func TestSetLifeEffectAboveCurrentIsAGain(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p := g.Players()[0]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life, g.Player(g.Players()[1]).Life = 5, 20

	c := engine.NewScriptedController()
	_, err := castETBSetLife(t, g, p, etbSetLifeTriggerDefParams(t, "Test SetLife Gain", "LifeAmount$ 12 | Defined$ You"), c)
	if err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if got := g.Player(p).Life; got != 12 {
		t.Errorf("p life = %d, want 12", got)
	}
	if got := g.Player(p).LifeGainedTimesThisTurn; got != 1 {
		t.Errorf("p LifeGainedTimesThisTurn = %d, want 1 -- a rise must route through the real gain path", got)
	}
}

// TestSetLifeEffectBelowCurrentIsALoss proves the mirror direction: a life
// total set below its current value is a plain loss, LifeGainedTimesThisTurn
// left untouched.
func TestSetLifeEffectBelowCurrentIsALoss(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p := g.Players()[0]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life, g.Player(g.Players()[1]).Life = 20, 20

	c := engine.NewScriptedController()
	_, err := castETBSetLife(t, g, p, etbSetLifeTriggerDefParams(t, "Test SetLife Loss", "LifeAmount$ 5 | Defined$ You"), c)
	if err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if got := g.Player(p).Life; got != 5 {
		t.Errorf("p life = %d, want 5", got)
	}
	if got := g.Player(p).LifeGainedTimesThisTurn; got != 0 {
		t.Errorf("p LifeGainedTimesThisTurn = %d, want 0 -- a drop is not a gain", got)
	}
}

// TestSetLifeEffectSameValueIsNoEvent proves CR 119.5's own explicit
// carve-out: setting a life total to its own current value causes no event
// -- neither Life nor LifeGainedTimesThisTurn moves.
func TestSetLifeEffectSameValueIsNoEvent(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p := g.Players()[0]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life, g.Player(g.Players()[1]).Life = 7, 20

	c := engine.NewScriptedController()
	_, err := castETBSetLife(t, g, p, etbSetLifeTriggerDefParams(t, "Test SetLife Same", "LifeAmount$ 7 | Defined$ You"), c)
	if err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if got := g.Player(p).Life; got != 7 {
		t.Errorf("p life = %d, want 7 (unchanged)", got)
	}
	if got := g.Player(p).LifeGainedTimesThisTurn; got != 0 {
		t.Errorf("p LifeGainedTimesThisTurn = %d, want 0", got)
	}
}

// TestSetLifeEffectValidTgtsSetsChosenPlayer proves an ability naming
// ValidTgts$ reads its own chosen player target directly
// (targetedOrDefinedPlayers, defined.go) rather than through Defined$.
func TestSetLifeEffectValidTgtsSetsChosenPlayer(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p, other := g.Players()[0], g.Players()[1]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life, g.Player(other).Life = 20, 20

	c := engine.NewScriptedController()
	c.QueueTargets([]engine.EntityID{engine.PlayerEntity(other)})
	_, err := castETBSetLife(t, g, p, etbSetLifeTriggerDefParams(t, "Test SetLife Tgt", "LifeAmount$ 1 | ValidTgts$ Player"), c)
	if err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if got := g.Player(other).Life; got != 1 {
		t.Errorf("other life = %d, want 1", got)
	}
}

// TestSetLifeEffectRejectsUnresolvedParam proves setLifeUnresolvedParams'
// own fail-loud contract (PORT-8/GO-7).
func TestSetLifeEffectRejectsUnresolvedParam(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p := g.Players()[0]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life, g.Player(g.Players()[1]).Life = 20, 20

	c := engine.NewScriptedController()
	_, err := castETBSetLife(t, g, p, etbSetLifeTriggerDefParams(t, "Test SetLife Ultimate", "LifeAmount$ 1 | Defined$ You | Ultimate$ True"), c)
	if err == nil {
		t.Fatal("ResolveStack succeeded, want an error for unresolved Ultimate$")
	}
}
