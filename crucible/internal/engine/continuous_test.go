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

// TestApplyContinuousTypeAddsTypeToMatchingCreatures proves Layer 4's own
// AddType$, the type-line counterpart to TestApplyContinuousPTAppliesAnthemToMatchingCreatures:
// a blanket "creatures you control are also Zombies" effect adds Zombie
// without displacing the creature's own printed Elf.
func TestApplyContinuousTypeAddsTypeToMatchingCreatures(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p, other := g.Players()[0], g.Players()[1]
	g.Player(p).Life, g.Player(other).Life = 20, 20
	g.NewCard(continuousDef(t, "Test Type Anthem", "Mode$ Continuous | Affected$ Creature.YouCtrl | AddType$ Zombie"), p, engine.Battlefield)
	creature := g.NewCard(creatureDefPT(t, "2", "2"), p, engine.Battlefield)

	engine.CheckStateBasedActions(g, engine.NewScriptedController())

	typ := g.Card(creature).Type()
	if !typ.HasSubtype("Zombie") {
		t.Errorf("Type() = %q, want it to carry the added Zombie subtype", typ)
	}
	if !typ.HasSubtype("Elf") {
		t.Errorf("Type() = %q, want it to still carry its own printed Elf subtype", typ)
	}
}

// TestApplyContinuousTypeDoesNotAffectNonMatchingCreatures mirrors
// TestApplyContinuousPTDoesNotAffectNonMatchingCreatures: an opponent's
// creature is untouched by a "creatures you control" type-granting anthem.
func TestApplyContinuousTypeDoesNotAffectNonMatchingCreatures(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p, other := g.Players()[0], g.Players()[1]
	g.Player(p).Life, g.Player(other).Life = 20, 20
	g.NewCard(continuousDef(t, "Test Type Anthem", "Mode$ Continuous | Affected$ Creature.YouCtrl | AddType$ Zombie"), p, engine.Battlefield)
	theirs := g.NewCard(creatureDefPT(t, "2", "2"), other, engine.Battlefield)

	engine.CheckStateBasedActions(g, engine.NewScriptedController())

	if typ := g.Card(theirs).Type(); typ.HasSubtype("Zombie") {
		t.Errorf("Type() = %q, an opponent's anthem must not add Zombie to it", typ)
	}
}

// TestApplyContinuousTypeRecomputesWhenSourceLeaves mirrors
// TestApplyContinuousPTRecomputesWhenSourceLeaves: once the type-granting
// source itself leaves the battlefield, the added type is gone on the very
// next check.
func TestApplyContinuousTypeRecomputesWhenSourceLeaves(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p, other := g.Players()[0], g.Players()[1]
	g.Player(p).Life, g.Player(other).Life = 20, 20
	anthem := g.NewCard(continuousDef(t, "Test Type Anthem", "Mode$ Continuous | Affected$ Creature.YouCtrl | AddType$ Zombie"), p, engine.Battlefield)
	creature := g.NewCard(creatureDefPT(t, "2", "2"), p, engine.Battlefield)

	engine.CheckStateBasedActions(g, engine.NewScriptedController())
	if typ := g.Card(creature).Type(); !typ.HasSubtype("Zombie") {
		t.Fatalf("setup: Type() = %q, want it to carry Zombie", typ)
	}

	g.Move(anthem, engine.Graveyard, p)
	engine.CheckStateBasedActions(g, engine.NewScriptedController())

	if typ := g.Card(creature).Type(); typ.HasSubtype("Zombie") {
		t.Errorf("Type() after the anthem left = %q, want Zombie gone", typ)
	}
}

// TestApplyContinuousTypeRemovesNamedType proves RemoveType$'s own plain
// literal-token shape: a real corpus "loses all creature types" line spelled
// as a single named RemoveType$ (not the bulk RemoveCreatureTypes$ flag)
// clears exactly that subtype and nothing else.
func TestApplyContinuousTypeRemovesNamedType(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p, other := g.Players()[0], g.Players()[1]
	g.Player(p).Life, g.Player(other).Life = 20, 20
	g.NewCard(continuousDef(t, "Test Type Remover", "Mode$ Continuous | Affected$ Creature.YouCtrl | RemoveType$ Elf"), p, engine.Battlefield)
	creature := g.NewCard(creatureDefPT(t, "2", "2"), p, engine.Battlefield)

	engine.CheckStateBasedActions(g, engine.NewScriptedController())

	typ := g.Card(creature).Type()
	if typ.HasSubtype("Elf") {
		t.Errorf("Type() = %q, want Elf removed", typ)
	}
	if !typ.Has(cardtype.Creature) {
		t.Errorf("Type() = %q, want the Creature core type left untouched", typ)
	}
}

// TestApplyContinuousTypeSkipsDynamicValue proves an AddType$ token this
// port cannot resolve at runtime (ChosenType, a chosen-type reference) skips
// the whole line rather than adding a literal subtype named "ChosenType".
func TestApplyContinuousTypeSkipsDynamicValue(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p, other := g.Players()[0], g.Players()[1]
	g.Player(p).Life, g.Player(other).Life = 20, 20
	g.NewCard(continuousDef(t, "Test Chosen Type Anthem", "Mode$ Continuous | Affected$ Creature.YouCtrl | AddType$ ChosenType"), p, engine.Battlefield)
	creature := g.NewCard(creatureDefPT(t, "2", "2"), p, engine.Battlefield)

	engine.CheckStateBasedActions(g, engine.NewScriptedController())

	if typ := g.Card(creature).Type(); typ.HasSubtype("ChosenType") {
		t.Errorf("Type() = %q, an unresolvable ChosenType token must not be added as a literal subtype", typ)
	}
}

// TestApplyContinuousTypeSkipsBulkRemovalFlag proves a line pairing AddType$
// with a bulk RemoveCreatureTypes$ flag (the real "becomes a Turtle" shape,
// StaticAbilityContinuous.java:425-448) is skipped whole: applying AddType$
// alone, without the wipe RemoveCreatureTypes$ asks for, would leave the
// creature with both its old and new creature types -- an actively wrong
// answer this port refuses to give rather than shipping half of a line.
func TestApplyContinuousTypeSkipsBulkRemovalFlag(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p, other := g.Players()[0], g.Players()[1]
	g.Player(p).Life, g.Player(other).Life = 20, 20
	g.NewCard(continuousDef(t, "Test Turtle Aura", "Mode$ Continuous | Affected$ Creature.YouCtrl | AddType$ Turtle | RemoveCreatureTypes$ True"), p, engine.Battlefield)
	creature := g.NewCard(creatureDefPT(t, "2", "2"), p, engine.Battlefield)

	engine.CheckStateBasedActions(g, engine.NewScriptedController())

	typ := g.Card(creature).Type()
	if typ.HasSubtype("Turtle") {
		t.Errorf("Type() = %q, want the whole line skipped (RemoveCreatureTypes$ is not evaluated), not just partially applied", typ)
	}
	if !typ.HasSubtype("Elf") {
		t.Errorf("Type() = %q, want the creature's own printed Elf left untouched by the skipped line", typ)
	}
}
