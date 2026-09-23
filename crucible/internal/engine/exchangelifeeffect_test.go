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

// etbExchangeLifeTriggerDefParams builds a *compile.Card whose own "when
// CARDNAME enters" trigger runs DB$ ExchangeLife with the given params
// string appended -- etbDestroyTriggerDefParams' own shape
// (destroyeffect_test.go).
func etbExchangeLifeTriggerDefParams(t *testing.T, name, extraParams string) *compile.Card {
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
		"Mode$ ChangesZone | Origin$ Any | Destination$ Battlefield | ValidCard$ Card.Self | Execute$ TrigExchange",
	}
	raw.Faces[0].SVars.Set("TrigExchange", "DB$ ExchangeLife | "+extraParams)

	c, err := compile.Compile(raw)
	if err != nil {
		t.Fatalf("compile %q: %v", name, err)
	}
	return c
}

// castETBExchangeLife casts def for p on controller c and resolves the
// stack -- castETBDestroy's own shape (destroyeffect_test.go).
func castETBExchangeLife(t *testing.T, g *engine.Game, p engine.PlayerID, def *compile.Card, c *engine.ScriptedController) (engine.CardID, error) {
	t.Helper()
	g.Player(p).ManaPool.Add(mana.Green, 1)
	creature := g.NewCard(def, p, engine.Hand)
	if !g.CastSpell(p, creature, c) {
		t.Fatal("CastSpell failed casting a creature with exactly enough mana")
	}
	return creature, g.ResolveStack(engine.NewRegistry(), c)
}

// TestExchangeLifeEffectOneTargetSwapsWithActivator proves the corpus's own
// dominant real shape: a single ValidTgts$ target exchanges life totals
// with the ability's own activator.
func TestExchangeLifeEffectOneTargetSwapsWithActivator(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p, other := g.Players()[0], g.Players()[1]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life, g.Player(other).Life = 3, 15

	c := engine.NewScriptedController()
	c.QueueTargets([]engine.EntityID{engine.PlayerEntity(other)})
	_, err := castETBExchangeLife(t, g, p, etbExchangeLifeTriggerDefParams(t, "Test Exchange One", "ValidTgts$ Opponent"), c)
	if err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if got := g.Player(p).Life; got != 15 {
		t.Errorf("p life = %d, want 15", got)
	}
	if got := g.Player(other).Life; got != 3 {
		t.Errorf("other life = %d, want 3", got)
	}
}

// TestExchangeLifeEffectTwoTargetsSwapWithEachOther proves the corpus's own
// second real shape: two ValidTgts$ targets exchange with each other,
// leaving the activator (a third player, uninvolved here) untouched.
func TestExchangeLifeEffectTwoTargetsSwapWithEachOther(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b", "c")
	activator, first, second := g.Players()[0], g.Players()[1], g.Players()[2]
	g.SetTurnState(1, activator, engine.Main1)
	g.Player(activator).Life = 20
	g.Player(first).Life = 8
	g.Player(second).Life = 22

	c := engine.NewScriptedController()
	c.QueueTargets([]engine.EntityID{engine.PlayerEntity(first), engine.PlayerEntity(second)})
	_, err := castETBExchangeLife(t, g, activator, etbExchangeLifeTriggerDefParams(t, "Test Exchange Two", "ValidTgts$ Player | TargetMin$ 2 | TargetMax$ 2"), c)
	if err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if got := g.Player(first).Life; got != 22 {
		t.Errorf("first life = %d, want 22", got)
	}
	if got := g.Player(second).Life; got != 8 {
		t.Errorf("second life = %d, want 8", got)
	}
	if got := g.Player(activator).Life; got != 20 {
		t.Errorf("activator life = %d, want 20 (uninvolved)", got)
	}
}

// TestExchangeLifeEffectEqualLifeIsNoOp proves the zero-difference no-op:
// two equal life totals have nothing to exchange.
func TestExchangeLifeEffectEqualLifeIsNoOp(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p, other := g.Players()[0], g.Players()[1]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life, g.Player(other).Life = 20, 20

	c := engine.NewScriptedController()
	c.QueueTargets([]engine.EntityID{engine.PlayerEntity(other)})
	_, err := castETBExchangeLife(t, g, p, etbExchangeLifeTriggerDefParams(t, "Test Exchange Equal", "ValidTgts$ Opponent"), c)
	if err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if got := g.Player(p).Life; got != 20 {
		t.Errorf("p life = %d, want 20", got)
	}
	if got := g.Player(other).Life; got != 20 {
		t.Errorf("other life = %d, want 20", got)
	}
}

// TestExchangeLifeEffectRejectsUnresolvedParam proves
// exchangeLifeUnresolvedParams' own fail-loud contract (PORT-8/GO-7).
func TestExchangeLifeEffectRejectsUnresolvedParam(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p, other := g.Players()[0], g.Players()[1]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life, g.Player(other).Life = 5, 15

	c := engine.NewScriptedController()
	c.QueueTargets([]engine.EntityID{engine.PlayerEntity(other)})
	_, err := castETBExchangeLife(t, g, p, etbExchangeLifeTriggerDefParams(t, "Test Exchange Remember", "ValidTgts$ Opponent | RememberDifference$ True"), c)
	if err == nil {
		t.Fatal("ResolveStack succeeded, want an error for unresolved RememberDifference$")
	}
}
