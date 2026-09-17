package engine_test

import (
	"strings"
	"testing"

	"github.com/jczastkiewicz/crucible/internal/carddb"
	"github.com/jczastkiewicz/crucible/internal/carddb/compile"
	"github.com/jczastkiewicz/crucible/internal/cardtype"
	"github.com/jczastkiewicz/crucible/internal/engine"
)

// continuousDef builds a *compile.Card for a non-creature permanent carrying
// one real S:Mode$ Continuous line, compiled through the real pipeline the
// same reason cantBlockByAuraDef (staticability_test.go) is. applyContinuousPT
// is unexported, so every case here is driven through CheckStateBasedActions,
// its only caller (TEST-1).
func continuousDef(t *testing.T, name, static string) *compile.Card {
	t.Helper()

	reg, err := cardtype.LoadRegistry(strings.NewReader("[CreatureTypes]\nElf\n"))
	if err != nil {
		t.Fatalf("LoadRegistry: %v", err)
	}
	raw := &carddb.Card{Filename: name}
	raw.Faces[0].Present = true
	raw.Faces[0].Name = name
	raw.Faces[0].Type = cardtype.Parse(reg, "Enchantment")
	raw.Faces[0].Statics = []string{static}

	c, err := compile.Compile(raw)
	if err != nil {
		t.Fatalf("compile %q: %v", name, err)
	}
	return c
}

// TestApplyContinuousPTAppliesAnthemToMatchingCreatures proves the
// corpus-frequent anthem shape (Affected$ Creature.YouCtrl, a blanket
// valid-string match) actually boosts every matching creature's own
// Power/Toughness once CheckStateBasedActions recomputes it.
func TestApplyContinuousPTAppliesAnthemToMatchingCreatures(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p, other := g.Players()[0], g.Players()[1]
	g.Player(p).Life, g.Player(other).Life = 20, 20
	g.NewCard(continuousDef(t, "Test Anthem", "Mode$ Continuous | Affected$ Creature.YouCtrl | AddPower$ 1 | AddToughness$ 1"), p, engine.Battlefield)
	creature := g.NewCard(creatureDefPT(t, "2", "2"), p, engine.Battlefield)

	engine.CheckStateBasedActions(g, engine.NewScriptedController())

	if pw, ok := g.Card(creature).Power(); !ok || pw != 3 {
		t.Errorf("Power() = (%d, %v), want (3, true)", pw, ok)
	}
	if tg, ok := g.Card(creature).Toughness(); !ok || tg != 3 {
		t.Errorf("Toughness() = (%d, %v), want (3, true)", tg, ok)
	}
}

// TestApplyContinuousPTDoesNotAffectNonMatchingCreatures proves the
// Affected$ restriction is actually checked, not applied blanket to every
// creature in the game: an opponent's creature is untouched by a "creatures
// you control" anthem.
func TestApplyContinuousPTDoesNotAffectNonMatchingCreatures(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p, other := g.Players()[0], g.Players()[1]
	g.Player(p).Life, g.Player(other).Life = 20, 20
	g.NewCard(continuousDef(t, "Test Anthem", "Mode$ Continuous | Affected$ Creature.YouCtrl | AddPower$ 1 | AddToughness$ 1"), p, engine.Battlefield)
	theirs := g.NewCard(creatureDefPT(t, "2", "2"), other, engine.Battlefield)

	engine.CheckStateBasedActions(g, engine.NewScriptedController())

	if pw, ok := g.Card(theirs).Power(); !ok || pw != 2 {
		t.Errorf("Power() = (%d, %v), want (2, true) -- an opponent's anthem must not affect it", pw, ok)
	}
}

// TestApplyContinuousPTRecomputesWhenSourceLeaves proves continuous effects
// are recalculated fresh every pass, not pushed once and left to persist:
// once the anthem itself leaves the battlefield, the creature it used to
// boost reverts to its printed stats on the very next check.
func TestApplyContinuousPTRecomputesWhenSourceLeaves(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p, other := g.Players()[0], g.Players()[1]
	g.Player(p).Life, g.Player(other).Life = 20, 20
	anthem := g.NewCard(continuousDef(t, "Test Anthem", "Mode$ Continuous | Affected$ Creature.YouCtrl | AddPower$ 1 | AddToughness$ 1"), p, engine.Battlefield)
	creature := g.NewCard(creatureDefPT(t, "2", "2"), p, engine.Battlefield)

	engine.CheckStateBasedActions(g, engine.NewScriptedController())
	if pw, _ := g.Card(creature).Power(); pw != 3 {
		t.Fatalf("setup: Power() = %d, want 3", pw)
	}

	g.Move(anthem, engine.Graveyard, p)
	engine.CheckStateBasedActions(g, engine.NewScriptedController())

	if pw, ok := g.Card(creature).Power(); !ok || pw != 2 {
		t.Errorf("Power() after the anthem left = (%d, %v), want (2, true)", pw, ok)
	}
}

// TestApplyContinuousPTSetPowerToughnessPartial proves a real corpus
// SetPower/SetToughness line naming only one dimension leaves the other
// exactly as it was -- PTEffect's own HasPower/HasToughness (pt.go), now
// exercised by a real caller for the first time.
func TestApplyContinuousPTSetPowerToughnessPartial(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p, other := g.Players()[0], g.Players()[1]
	g.Player(p).Life, g.Player(other).Life = 20, 20
	g.NewCard(continuousDef(t, "Test SetPower", "Mode$ Continuous | Affected$ Creature.YouCtrl | SetPower$ 0"), p, engine.Battlefield)
	creature := g.NewCard(creatureDefPT(t, "4", "4"), p, engine.Battlefield)

	engine.CheckStateBasedActions(g, engine.NewScriptedController())

	if pw, ok := g.Card(creature).Power(); !ok || pw != 0 {
		t.Errorf("Power() = (%d, %v), want (0, true)", pw, ok)
	}
	if tg, ok := g.Card(creature).Toughness(); !ok || tg != 4 {
		t.Errorf("Toughness() = (%d, %v), want (4, true) -- SetPower alone must not touch Toughness", tg, ok)
	}
}

// TestApplyContinuousPTSkipsConditionParam proves a line carrying Condition$
// -- a runtime gate this port cannot evaluate for any static-ability mode --
// is skipped entirely rather than treated as always active.
func TestApplyContinuousPTSkipsConditionParam(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p, other := g.Players()[0], g.Players()[1]
	g.Player(p).Life, g.Player(other).Life = 20, 20
	g.NewCard(continuousDef(t, "Test Conditional Anthem", "Mode$ Continuous | Affected$ Creature.YouCtrl | AddPower$ 1 | AddToughness$ 1 | Condition$ PlayerTurn"), p, engine.Battlefield)
	creature := g.NewCard(creatureDefPT(t, "2", "2"), p, engine.Battlefield)

	engine.CheckStateBasedActions(g, engine.NewScriptedController())

	if pw, ok := g.Card(creature).Power(); !ok || pw != 2 {
		t.Errorf("Power() = (%d, %v), want (2, true) -- Condition$ is not evaluated, so this must not apply", pw, ok)
	}
}

// TestApplyContinuousPTSkipsNonNumericValue proves an SVar-driven AddPower$
// (X, Y, a named SVar) is skipped rather than resolved to zero or crashing.
func TestApplyContinuousPTSkipsNonNumericValue(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p, other := g.Players()[0], g.Players()[1]
	g.Player(p).Life, g.Player(other).Life = 20, 20
	g.NewCard(continuousDef(t, "Test X Anthem", "Mode$ Continuous | Affected$ Creature.YouCtrl | AddPower$ X | AddToughness$ X"), p, engine.Battlefield)
	creature := g.NewCard(creatureDefPT(t, "2", "2"), p, engine.Battlefield)

	engine.CheckStateBasedActions(g, engine.NewScriptedController())

	if pw, ok := g.Card(creature).Power(); !ok || pw != 2 {
		t.Errorf("Power() = (%d, %v), want (2, true) -- a non-numeric AddPower$ must not apply", pw, ok)
	}
}
