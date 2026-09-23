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

// etbRemoveCounterAllTriggerDefParams builds a *compile.Card whose own
// "when CARDNAME enters" trigger runs DB$ RemoveCounterAll with the given
// params string appended -- etbDestroyTriggerDefParams' own shape
// (destroyeffect_test.go).
func etbRemoveCounterAllTriggerDefParams(t *testing.T, name, extraParams string) *compile.Card {
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
		"Mode$ ChangesZone | Origin$ Any | Destination$ Battlefield | ValidCard$ Card.Self | Execute$ TrigRemoveCounterAll",
	}
	raw.Faces[0].SVars.Set("TrigRemoveCounterAll", "DB$ RemoveCounterAll | "+extraParams)

	c, err := compile.Compile(raw)
	if err != nil {
		t.Fatalf("compile %q: %v", name, err)
	}
	return c
}

// castETBRemoveCounterAll casts def for p on controller c and resolves the
// stack -- castETBDestroy's own shape (destroyeffect_test.go).
func castETBRemoveCounterAll(t *testing.T, g *engine.Game, p engine.PlayerID, def *compile.Card, c *engine.ScriptedController) (engine.CardID, error) {
	t.Helper()
	g.Player(p).ManaPool.Add(mana.Green, 1)
	creature := g.NewCard(def, p, engine.Hand)
	if !g.CastSpell(p, creature, c) {
		t.Fatal("CastSpell failed casting a creature with exactly enough mana")
	}
	return creature, g.ResolveStack(engine.NewRegistry(), c)
}

// TestRemoveCounterAllEffectRemovesLiteralAmountFromEveryMatch proves the
// corpus's own dominant real shape: a literal CounterNum$ removed from
// every ValidCards$ match across the whole battlefield.
func TestRemoveCounterAllEffectRemovesLiteralAmountFromEveryMatch(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p, other := g.Players()[0], g.Players()[1]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life, g.Player(other).Life = 20, 20
	mine := g.NewCard(creatureDefPT(t, "2", "2"), p, engine.Battlefield)
	theirs := g.NewCard(creatureDefPT(t, "2", "2"), other, engine.Battlefield)
	g.Card(mine).Counters.Add(engine.P1P1, 3)
	g.Card(theirs).Counters.Add(engine.P1P1, 3)

	c := engine.NewScriptedController()
	_, err := castETBRemoveCounterAll(t, g, p, etbRemoveCounterAllTriggerDefParams(t, "Test RemoveCounterAll Literal", "ValidCards$ Creature.StrictlyOther | CounterType$ P1P1 | CounterNum$ 2"), c)
	if err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if n := g.Card(mine).Counters.Count(engine.P1P1); n != 1 {
		t.Errorf("mine P1P1 = %d, want 1 (3 - 2)", n)
	}
	if n := g.Card(theirs).Counters.Count(engine.P1P1); n != 1 {
		t.Errorf("theirs P1P1 = %d, want 1 (3 - 2)", n)
	}
}

// TestRemoveCounterAllEffectAllCountersRemovesEachTargetsOwnCurrentCount
// proves AllCounters$ reads each target's own current count individually --
// two targets with different starting counts both end at zero.
func TestRemoveCounterAllEffectAllCountersRemovesEachTargetsOwnCurrentCount(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p, other := g.Players()[0], g.Players()[1]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life, g.Player(other).Life = 20, 20
	mine := g.NewCard(creatureDefPT(t, "2", "2"), p, engine.Battlefield)
	theirs := g.NewCard(creatureDefPT(t, "2", "2"), other, engine.Battlefield)
	g.Card(mine).Counters.Add(engine.P1P1, 2)
	g.Card(theirs).Counters.Add(engine.P1P1, 5)

	c := engine.NewScriptedController()
	_, err := castETBRemoveCounterAll(t, g, p, etbRemoveCounterAllTriggerDefParams(t, "Test RemoveCounterAll AllCounters", "ValidCards$ Creature.StrictlyOther | CounterType$ P1P1 | AllCounters$ True"), c)
	if err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if n := g.Card(mine).Counters.Count(engine.P1P1); n != 0 {
		t.Errorf("mine P1P1 = %d, want 0", n)
	}
	if n := g.Card(theirs).Counters.Count(engine.P1P1); n != 0 {
		t.Errorf("theirs P1P1 = %d, want 0", n)
	}
}

// TestRemoveCounterAllEffectRejectsUnresolvedParam proves
// removeCounterAllUnresolvedParams' own fail-loud contract (PORT-8/GO-7).
func TestRemoveCounterAllEffectRejectsUnresolvedParam(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p := g.Players()[0]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life, g.Player(g.Players()[1]).Life = 20, 20

	c := engine.NewScriptedController()
	_, err := castETBRemoveCounterAll(t, g, p, etbRemoveCounterAllTriggerDefParams(t, "Test RemoveCounterAll Zone", "ValidCards$ Creature | CounterType$ P1P1 | ValidZone$ Graveyard"), c)
	if err == nil {
		t.Fatal("ResolveStack succeeded, want an error for unresolved ValidZone$")
	}
}
