package engine_test

import (
	"strings"
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
)

// StaticEffect$ (GameAction.setupStaticEffect): the card the ChangeZone moved
// takes the Mode$ Continuous static its SVar names, and only that card.
func TestChangeZoneStaticEffectChangesTheMovedCard(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGame(t)
	moved := g.NewCard(creatureDefPT(t, "1", "1"), p, engine.Library)
	other := g.NewCard(creatureDefPT(t, "1", "1"), p, engine.Battlefield)
	c := engine.NewScriptedController()
	c.QueueCardChoice([]engine.CardID{moved})
	line := "DB$ ChangeZone | Origin$ Library | Destination$ Battlefield | ChangeType$ Card | Mandatory$ True | StaticEffect$ Animate"
	def := etbChainDef(t, "Test Static Effect", line,
		"Animate", "Mode$ Continuous | Affected$ Card.IsRemembered | SetPower$ 3 | SetToughness$ 4 | AddKeyword$ Flying")
	if _, err := castETBChain(t, g, p, def, c); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if pw, ok := g.Card(moved).Power(); !ok || pw != 3 {
		t.Errorf("moved power = %d, want 3", pw)
	}
	if tg, _ := g.Card(moved).Toughness(); tg != 4 {
		t.Errorf("moved toughness = %d, want 4", tg)
	}
	if !g.Card(moved).HasKeyword("Flying") {
		t.Error("moved card lacks Flying")
	}
	if pw, _ := g.Card(other).Power(); pw != 1 {
		t.Errorf("other power = %d, want 1: the static applies to the moved card only", pw)
	}
}

// A static this port cannot apply refuses the line before anything moves.
func TestChangeZoneStaticEffectRefusesAnUnreadStatic(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGame(t)
	moved := g.NewCard(creatureDefPT(t, "1", "1"), p, engine.Library)
	c := engine.NewScriptedController()
	c.QueueCardChoice([]engine.CardID{moved})
	line := "DB$ ChangeZone | Origin$ Library | Destination$ Battlefield | ChangeType$ Card | Mandatory$ True | StaticEffect$ Animate"
	def := etbChainDef(t, "Test Static Effect Bad", line,
		"Animate", "Mode$ Continuous | Affected$ Card.IsRemembered | AddHiddenKeyword$ Bogus")
	_, err := castETBChain(t, g, p, def, c)
	if err == nil || !strings.Contains(err.Error(), "StaticEffect$") {
		t.Fatalf("err = %v, want one naming StaticEffect$", err)
	}
	if z := g.Card(moved).Zone; z != engine.Library {
		t.Errorf("card zone = %v, want Library", z)
	}
}

// ChangeZoneAll honours StaticEffect$ through the same moveTo path
// (Grimoire of the Dead): every card it moves takes the static.
func TestChangeZoneAllStaticEffectChangesEveryMovedCard(t *testing.T) {
	t.Parallel()

	g, p, other := newTwoPlayerGame(t)
	mine := g.NewCard(creatureDefPT(t, "1", "1"), p, engine.Graveyard)
	theirs := g.NewCard(creatureDefPT(t, "1", "1"), other, engine.Graveyard)
	line := "DB$ ChangeZoneAll | ChangeType$ Creature | Origin$ Graveyard | Destination$ Battlefield | StaticEffect$ Animate"
	def := etbChainDef(t, "Test Static All", line,
		"Animate", "Mode$ Continuous | Affected$ Card.IsRemembered | SetPower$ 3 | SetToughness$ 4")
	if _, err := castETBChain(t, g, p, def, engine.NewScriptedController()); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	for _, id := range []engine.CardID{mine, theirs} {
		if pw, _ := g.Card(id).Power(); pw != 3 {
			t.Errorf("card %v power = %d, want 3", id, pw)
		}
	}
}

// StaticEffectCheckSVar$ with StaticEffectSVarCompare$ (Dance of the Manse,
// GameAction.java:686-694) gates the static: the moved card takes it only when
// the check holds.
func TestChangeZoneStaticEffectCheckSVarGatesTheStatic(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		x     string
		power int
	}{{"Number$6", 3}, {"Number$5", 1}} {
		g, p, _ := newTwoPlayerGame(t)
		moved := g.NewCard(creatureDefPT(t, "1", "1"), p, engine.Library)
		c := engine.NewScriptedController()
		c.QueueCardChoice([]engine.CardID{moved})
		line := "DB$ ChangeZone | Origin$ Library | Destination$ Battlefield | ChangeType$ Card | Mandatory$ True | StaticEffect$ Animate | StaticEffectCheckSVar$ X | StaticEffectSVarCompare$ GE6"
		def := etbChainDef(t, "Test Static Check", line,
			"Animate", "Mode$ Continuous | Affected$ Card.IsRemembered | SetPower$ 3 | SetToughness$ 4",
			"X", tc.x)
		if _, err := castETBChain(t, g, p, def, c); err != nil {
			t.Fatalf("X %s: ResolveStack: %v", tc.x, err)
		}
		if pw, _ := g.Card(moved).Power(); pw != tc.power {
			t.Errorf("X %s: moved power = %d, want %d", tc.x, pw, tc.power)
		}
	}
}
