package engine_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/jczastkiewicz/crucible/internal/carddb"
	"github.com/jczastkiewicz/crucible/internal/carddb/compile"
	"github.com/jczastkiewicz/crucible/internal/cardtype"
	"github.com/jczastkiewicz/crucible/internal/engine"
	"github.com/jczastkiewicz/crucible/internal/mana"
)

// etbTriggerCreatureDef builds a *compile.Card for a creature with a real
// "when CARDNAME enters" trigger -- Elvish Visionary's own shape
// (Mode$ ChangesZone, Destination$ Battlefield, ValidCard$ Card.Self,
// Execute$ referencing a DB$ Draw sub-ability), compiled through the real
// pipeline (compile.Compile) rather than hand-built field by field, so a
// change to how M3 compiles a T:/SVar: pair into compile.Ability/SubRef would
// break this test the same way it would break any other card.
func etbTriggerCreatureDef(t *testing.T, name, cost string) *compile.Card {
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
	raw.Faces[0].ManaCost = mana.MustParse(cost)
	raw.Faces[0].Triggers = []string{
		"Mode$ ChangesZone | Origin$ Any | Destination$ Battlefield | ValidCard$ Card.Self | Execute$ TrigDraw",
	}
	raw.Faces[0].SVars.Set("TrigDraw", "DB$ Draw | Defined$ You | NumCards$ 1")

	c, err := compile.Compile(raw)
	if err != nil {
		t.Fatalf("compile %q: %v", name, err)
	}
	return c
}

// TestCastSpellFiresETBTrigger casts a creature carrying a real "when
// CARDNAME enters, draw a card" trigger and proves checkETBTriggers
// (trigger.go) detects it and pushes its own Execute$ sub-ability (Draw)
// onto the stack -- ResolveStack resolving the permanent spell itself
// succeeds (the creature reaches the battlefield), then immediately finds
// the pushed Draw ability on top and reports ErrUnimplemented for it, since
// Draw is one of the 203 corpus-frequency effects M6 still owns
// (effect.go's own Registry contract). Detecting and queuing the trigger
// correctly -- not resolving it -- is this port's job today.
//
// Two players, not one, and both with Life set explicitly: CheckStateBasedActions
// (run after every resolved ability, stack.go) declares a lone remaining
// player the winner and sets Game.Over the moment it finds exactly one --
// true in a one-player game as soon as the first ability resolves, and just
// as true in a two-player game if either player's own Life is left at
// NewGame's own zero value (0 <= 0 loses immediately, action.go). Either
// way, Over stops ResolveStack's own loop (`for len(g.stack) > 0 &&
// !g.over`) before it ever reaches the pushed trigger, and this test would
// pass for the wrong reason: not because the trigger correctly failed to
// resolve, but because the game ended first.
func TestCastSpellFiresETBTrigger(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p, other := g.Players()[0], g.Players()[1]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life, g.Player(other).Life = 20, 20
	g.Player(p).ManaPool.Add(mana.Green, 1)
	g.Player(p).ManaPool.AddColorless(1)
	creature := g.NewCard(etbTriggerCreatureDef(t, "Test Visionary", "1 G"), p, engine.Hand)
	c := engine.NewScriptedController()
	c.QueuePayGeneric(mana.ShardC)

	if !g.CastSpell(p, creature, c) {
		t.Fatal("CastSpell failed casting a creature with exactly enough mana")
	}

	err := g.ResolveStack(engine.NewRegistry(), c)
	if !errors.Is(err, engine.ErrUnimplemented) {
		t.Fatalf("ResolveStack error = %v, want ErrUnimplemented (Draw)", err)
	}
	if !strings.Contains(err.Error(), "Draw") {
		t.Errorf("ResolveStack error = %q, want it to name Draw", err.Error())
	}
	if g.Card(creature).Zone != engine.Battlefield {
		t.Errorf("creature zone = %v, want Battlefield -- the permanent spell itself should still have resolved", g.Card(creature).Zone)
	}
	if g.StackLen() != 0 {
		t.Errorf("StackLen() = %d, want 0 -- the failed trigger was popped before its own Resolve ran", g.StackLen())
	}
}

// A trigger whose ValidCard does not match the card that entered never
// pushes anything -- Matches (valid.go) doing its job, the same evaluator
// every other trigger-adjacent check (state-based actions, enchantSpec)
// already trusts.
func TestCastSpellSkipsNonMatchingTrigger(t *testing.T) {
	t.Parallel()

	reg, err := cardtype.LoadRegistry(strings.NewReader("[CreatureTypes]\nElf\n"))
	if err != nil {
		t.Fatalf("LoadRegistry: %v", err)
	}
	raw := &carddb.Card{Filename: "Test Watcher"}
	raw.Faces[0].Present = true
	raw.Faces[0].Name = "Test Watcher"
	raw.Faces[0].Type = cardtype.Parse(reg, "Creature Elf")
	raw.Faces[0].Power, raw.Faces[0].Toughness = "1", "1"
	raw.Faces[0].ManaCost = mana.MustParse("G")
	// ValidCard$ Card.Wolf never matches this card (an Elf), so the trigger
	// this port checks (self-ETB only, trigger.go's own doc comment) never
	// fires for it.
	raw.Faces[0].Triggers = []string{
		"Mode$ ChangesZone | Origin$ Any | Destination$ Battlefield | ValidCard$ Card.Wolf | Execute$ TrigDraw",
	}
	raw.Faces[0].SVars.Set("TrigDraw", "DB$ Draw | Defined$ You | NumCards$ 1")
	def, err := compile.Compile(raw)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}

	g := newGame(t, "a")
	p := g.Players()[0]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).ManaPool.Add(mana.Green, 1)
	creature := g.NewCard(def, p, engine.Hand)
	c := engine.NewScriptedController()

	if !g.CastSpell(p, creature, c) {
		t.Fatal("CastSpell failed casting a creature with exactly enough mana")
	}
	if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if g.Card(creature).Zone != engine.Battlefield {
		t.Errorf("creature zone = %v, want Battlefield", g.Card(creature).Zone)
	}
}
