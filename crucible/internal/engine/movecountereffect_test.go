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

// etbMoveCounterTriggerDefParams builds a *compile.Card whose own "when
// CARDNAME enters" trigger runs DB$ MoveCounter with the given params
// string appended -- etbDestroyTriggerDefParams' own shape
// (destroyeffect_test.go).
func etbMoveCounterTriggerDefParams(t *testing.T, name, extraParams string) *compile.Card {
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
		"Mode$ ChangesZone | Origin$ Any | Destination$ Battlefield | ValidCard$ Card.Self | Execute$ TrigMoveCounter",
	}
	raw.Faces[0].SVars.Set("TrigMoveCounter", "DB$ MoveCounter | "+extraParams)

	c, err := compile.Compile(raw)
	if err != nil {
		t.Fatalf("compile %q: %v", name, err)
	}
	return c
}

// castETBMoveCounter casts def for p on controller c and resolves the stack
// -- castETBDestroy's own shape (destroyeffect_test.go).
func castETBMoveCounter(t *testing.T, g *engine.Game, p engine.PlayerID, def *compile.Card, c *engine.ScriptedController) (engine.CardID, error) {
	t.Helper()
	g.Player(p).ManaPool.Add(mana.Green, 1)
	creature := g.NewCard(def, p, engine.Hand)
	if !g.CastSpell(p, creature, c) {
		t.Fatal("CastSpell failed casting a creature with exactly enough mana")
	}
	return creature, g.ResolveStack(engine.NewRegistry(), c)
}

// TestMoveCounterEffectSourceSelfMovesLiteralAmountToTarget proves the
// corpus's own dominant real shape: Source$ Self (default) moves a literal
// CounterNum$ of a literal CounterType$ to a ValidTgts$ destination.
func TestMoveCounterEffectSourceSelfMovesLiteralAmountToTarget(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p := g.Players()[0]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life, g.Player(g.Players()[1]).Life = 20, 20
	dest := g.NewCard(creatureDefPT(t, "2", "2"), p, engine.Battlefield)

	def := etbMoveCounterTriggerDefParams(t, "Test Move Literal", "CounterType$ P1P1 | CounterNum$ 2 | ValidTgts$ Creature")
	c := engine.NewScriptedController()
	c.QueueTargets([]engine.EntityID{engine.CardEntity(dest)})
	g.Player(p).ManaPool.Add(mana.Green, 1)
	host := g.NewCard(def, p, engine.Hand)
	// preset the host's own counters while it still sits in Hand -- Counters
	// is a plain per-card field, not zone-gated, castETBRemoveCounterSelf's
	// own identical idiom (removecountereffect_test.go).
	g.Card(host).Counters.Add(engine.P1P1, 3)
	if !g.CastSpell(p, host, c) {
		t.Fatal("CastSpell failed casting a creature with exactly enough mana")
	}
	if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if n := g.Card(host).Counters.Count(engine.P1P1); n != 1 {
		t.Errorf("host P1P1 = %d, want 1 (3 - 2)", n)
	}
	if n := g.Card(dest).Counters.Count(engine.P1P1); n != 2 {
		t.Errorf("dest P1P1 = %d, want 2", n)
	}
}

// TestMoveCounterEffectCounterNumAllMovesEntirePile proves CounterNum$ All
// moves every counter of the named kind, not just one.
func TestMoveCounterEffectCounterNumAllMovesEntirePile(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p := g.Players()[0]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life, g.Player(g.Players()[1]).Life = 20, 20
	dest := g.NewCard(creatureDefPT(t, "2", "2"), p, engine.Battlefield)

	def := etbMoveCounterTriggerDefParams(t, "Test Move All", "CounterType$ P1P1 | CounterNum$ All | ValidTgts$ Creature")
	c := engine.NewScriptedController()
	c.QueueTargets([]engine.EntityID{engine.CardEntity(dest)})
	g.Player(p).ManaPool.Add(mana.Green, 1)
	host := g.NewCard(def, p, engine.Hand)
	g.Card(host).Counters.Add(engine.P1P1, 5)
	if !g.CastSpell(p, host, c) {
		t.Fatal("CastSpell failed casting a creature with exactly enough mana")
	}
	if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if n := g.Card(host).Counters.Count(engine.P1P1); n != 0 {
		t.Errorf("host P1P1 = %d, want 0", n)
	}
	if n := g.Card(dest).Counters.Count(engine.P1P1); n != 5 {
		t.Errorf("dest P1P1 = %d, want 5", n)
	}
}

// TestMoveCounterEffectCounterTypeAllMovesEveryKind proves CounterType$ All
// moves every kind the source carries, each independently capped at its own
// count.
func TestMoveCounterEffectCounterTypeAllMovesEveryKind(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p := g.Players()[0]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life, g.Player(g.Players()[1]).Life = 20, 20
	dest := g.NewCard(creatureDefPT(t, "2", "2"), p, engine.Battlefield)

	def := etbMoveCounterTriggerDefParams(t, "Test Move AllKinds", "CounterType$ All | CounterNum$ All | ValidTgts$ Creature")
	c := engine.NewScriptedController()
	c.QueueTargets([]engine.EntityID{engine.CardEntity(dest)})
	g.Player(p).ManaPool.Add(mana.Green, 1)
	host := g.NewCard(def, p, engine.Hand)
	g.Card(host).Counters.Add(engine.P1P1, 2)
	g.Card(host).Counters.Add(engine.Stun, 1)
	if !g.CastSpell(p, host, c) {
		t.Fatal("CastSpell failed casting a creature with exactly enough mana")
	}
	if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if n := g.Card(dest).Counters.Count(engine.P1P1); n != 2 {
		t.Errorf("dest P1P1 = %d, want 2", n)
	}
	if n := g.Card(dest).Counters.Count(engine.Stun); n != 1 {
		t.Errorf("dest Stun = %d, want 1", n)
	}
}

// TestMoveCounterEffectRejectsUnresolvedParam proves
// moveCounterUnresolvedParams' own fail-loud contract (PORT-8/GO-7).
func TestMoveCounterEffectRejectsUnresolvedParam(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p := g.Players()[0]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life, g.Player(g.Players()[1]).Life = 20, 20
	dest1 := g.NewCard(creatureDefPT(t, "2", "2"), p, engine.Battlefield)
	dest2 := g.NewCard(creatureDefPT(t, "2", "2"), p, engine.Battlefield)

	def := etbMoveCounterTriggerDefParams(t, "Test Move TwoTarget", "CounterType$ P1P1 | CounterNum$ 1 | ValidTgts$ Creature | TargetMin$ 2 | TargetMax$ 2")
	c := engine.NewScriptedController()
	c.QueueTargets([]engine.EntityID{engine.CardEntity(dest1), engine.CardEntity(dest2)})
	g.Player(p).ManaPool.Add(mana.Green, 1)
	host := g.NewCard(def, p, engine.Hand)
	if !g.CastSpell(p, host, c) {
		t.Fatal("CastSpell failed casting a creature with exactly enough mana")
	}
	if err := g.ResolveStack(engine.NewRegistry(), c); err == nil {
		t.Fatal("ResolveStack succeeded, want an error for unresolved TargetMin$")
	}
}
