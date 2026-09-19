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

// continuousDefWithSVar is continuousDef plus one SVar the static line's own
// AddPower$/SetPower$/etc names -- resolveAmount's own real corpus shape
// (amount.go), compiled through the real pipeline so compile.Face.Amounts is
// actually built, not hand-constructed.
func continuousDefWithSVar(t *testing.T, name, static, svarName, svarBody string) *compile.Card {
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
	raw.Faces[0].SVars.Set(svarName, svarBody)

	c, err := compile.Compile(raw)
	if err != nil {
		t.Fatalf("compile %q: %v", name, err)
	}
	return c
}

// TestApplyContinuousPTResolvesNamedSVarCountValid proves resolveAmount
// (amount.go) closes the gap TestApplyContinuousPTSkipsNonNumericValue
// documents, for the one shape it actually can: AddPower$/AddToughness$
// naming an SVar whose own body is Count$Valid <spec> -- here, the number
// of Elves on the battlefield, which includes the anthem's own host (an
// Enchantment, not an Elf, so it does not count itself) and the one
// creature.
func TestApplyContinuousPTResolvesNamedSVarCountValid(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p, other := g.Players()[0], g.Players()[1]
	g.Player(p).Life, g.Player(other).Life = 20, 20
	g.NewCard(continuousDefWithSVar(t, "Test X Anthem",
		"Mode$ Continuous | Affected$ Creature.YouCtrl | AddPower$ X | AddToughness$ X",
		"X", "Count$Valid Elf"), p, engine.Battlefield)
	creature := g.NewCard(creatureDefPT(t, "2", "2"), p, engine.Battlefield)

	engine.CheckStateBasedActions(g, engine.NewScriptedController())

	if pw, ok := g.Card(creature).Power(); !ok || pw != 3 {
		t.Errorf("Power() = (%d, %v), want (3, true) -- AddPower$ X should resolve to 1, the one Elf on the battlefield", pw, ok)
	}
	if tg, ok := g.Card(creature).Toughness(); !ok || tg != 3 {
		t.Errorf("Toughness() = (%d, %v), want (3, true)", tg, ok)
	}
}

// TestApplyContinuousCharacteristicDefiningSetsFromCountValid proves Layer
// 7a: a CharacteristicDefining$ True line's own SetPower$/SetToughness$,
// naming an SVar whose body is Count$Valid <spec>, sets the host's own
// power/toughness to the number of matches -- reckless_one.txt's own real
// shape ("CARDNAME's power and toughness are each equal to the number of
// Goblins on the battlefield"), Elf standing in for Goblin here. Applies to
// the host itself only, not blanket to every Elf -- a second Elf on the
// battlefield is unaffected, proving Affected$ is not read for this shape
// (applyOneCharacteristicDefiningPT's own doc comment).
func TestApplyContinuousCharacteristicDefiningSetsFromCountValid(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p, other := g.Players()[0], g.Players()[1]
	g.Player(p).Life, g.Player(other).Life = 20, 20
	reg, err := cardtype.LoadRegistry(strings.NewReader("[CreatureTypes]\nElf\n"))
	if err != nil {
		t.Fatalf("LoadRegistry: %v", err)
	}
	raw := &carddb.Card{Filename: "Test CDA Elf"}
	raw.Faces[0].Present = true
	raw.Faces[0].Name = "Test CDA Elf"
	raw.Faces[0].Type = cardtype.Parse(reg, "Creature Elf")
	raw.Faces[0].Power, raw.Faces[0].Toughness = "0", "0"
	raw.Faces[0].Statics = []string{"Mode$ Continuous | CharacteristicDefining$ True | SetPower$ X | SetToughness$ X"}
	raw.Faces[0].SVars.Set("X", "Count$Valid Elf")
	def, err := compile.Compile(raw)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	cda := g.NewCard(def, p, engine.Battlefield)
	other2 := g.NewCard(creatureDefPT(t, "1", "1"), p, engine.Battlefield)

	engine.CheckStateBasedActions(g, engine.NewScriptedController())

	if pw, ok := g.Card(cda).Power(); !ok || pw != 2 {
		t.Errorf("Power() = (%d, %v), want (2, true) -- 2 Elves on the battlefield (CDA host + creatureDefPT)", pw, ok)
	}
	if tg, ok := g.Card(cda).Toughness(); !ok || tg != 2 {
		t.Errorf("Toughness() = (%d, %v), want (2, true)", tg, ok)
	}
	if pw, ok := g.Card(other2).Power(); !ok || pw != 1 {
		t.Errorf("other Elf's Power() = (%d, %v), want (1, true) -- CharacteristicDefining only ever describes its own host", pw, ok)
	}
}

// TestApplyContinuousPTResolvesChainedSVarReference proves resolveAmount's
// own Reference case: AddPower$ X, where X's own body is just "Y" (another
// SVar name, no Count$ of its own) and Y's is Count$Valid Elf -- one level
// of SVar-to-SVar indirection on top of the Expression case
// TestApplyContinuousPTResolvesNamedSVarCountValid already proves.
func TestApplyContinuousPTResolvesChainedSVarReference(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p, other := g.Players()[0], g.Players()[1]
	g.Player(p).Life, g.Player(other).Life = 20, 20
	reg, err := cardtype.LoadRegistry(strings.NewReader("[CreatureTypes]\nElf\n"))
	if err != nil {
		t.Fatalf("LoadRegistry: %v", err)
	}
	raw := &carddb.Card{Filename: "Test Chained SVar Anthem"}
	raw.Faces[0].Present = true
	raw.Faces[0].Name = "Test Chained SVar Anthem"
	raw.Faces[0].Type = cardtype.Parse(reg, "Enchantment")
	raw.Faces[0].Statics = []string{"Mode$ Continuous | Affected$ Creature.YouCtrl | AddPower$ X | AddToughness$ X"}
	raw.Faces[0].SVars.Set("X", "Y")
	raw.Faces[0].SVars.Set("Y", "Count$Valid Elf")
	def, err := compile.Compile(raw)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	g.NewCard(def, p, engine.Battlefield)
	creature := g.NewCard(creatureDefPT(t, "2", "2"), p, engine.Battlefield)

	engine.CheckStateBasedActions(g, engine.NewScriptedController())

	if pw, ok := g.Card(creature).Power(); !ok || pw != 3 {
		t.Errorf("Power() = (%d, %v), want (3, true) -- AddPower$ X should chain X->Y->Count$Valid Elf to 1", pw, ok)
	}
}

// TestApplyContinuousPTSkipsCountWithOperator proves a Count$ expression
// carrying an operator suffix (/Plus.1) is left unresolved rather than
// applied with the operator silently ignored -- resolveAmount's own doc
// comment names this as out of scope (needs its own operand evaluation,
// itself sometimes another SVar reference, Roiling Horror's own real shape).
func TestApplyContinuousPTSkipsCountWithOperator(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p, other := g.Players()[0], g.Players()[1]
	g.Player(p).Life, g.Player(other).Life = 20, 20
	g.NewCard(continuousDefWithSVar(t, "Test Operator Anthem",
		"Mode$ Continuous | Affected$ Creature.YouCtrl | AddPower$ X | AddToughness$ X",
		"X", "Count$Valid Elf/Plus.1"), p, engine.Battlefield)
	creature := g.NewCard(creatureDefPT(t, "2", "2"), p, engine.Battlefield)

	engine.CheckStateBasedActions(g, engine.NewScriptedController())

	if pw, ok := g.Card(creature).Power(); !ok || pw != 2 {
		t.Errorf("Power() = (%d, %v), want (2, true) -- an operator suffix must not resolve", pw, ok)
	}
}

// TestApplyContinuousPTResolvesMultiZoneCount proves a Count$Valid<Zone1>,
// <Zone2> head (a real corpus shape, 40-some lines naming more than one
// zone) counts across every zone it names, not just the first: one matching
// card on the battlefield and one in the graveyard both count.
func TestApplyContinuousPTResolvesMultiZoneCount(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p, other := g.Players()[0], g.Players()[1]
	g.Player(p).Life, g.Player(other).Life = 20, 20
	g.NewCard(continuousDefWithSVar(t, "Test Multi-Zone Anthem",
		"Mode$ Continuous | Affected$ Creature.YouCtrl | AddPower$ X | AddToughness$ X",
		"X", "Count$ValidGraveyard,Battlefield Creature.YouOwn"), p, engine.Battlefield)
	creature := g.NewCard(creatureDefPT(t, "2", "2"), p, engine.Battlefield)
	g.NewCard(creatureDefPT(t, "1", "1"), p, engine.Graveyard)

	engine.CheckStateBasedActions(g, engine.NewScriptedController())

	if pw, ok := g.Card(creature).Power(); !ok || pw != 4 {
		t.Errorf("Power() = (%d, %v), want (4, true) -- AddPower$ X should count both the battlefield creature and the graveyard one", pw, ok)
	}
}

// TestApplyContinuousCharacteristicDefiningSkipsUnresolvableSVar proves a
// CharacteristicDefining$ line whose own SetPower$/SetToughness$ SVar is not
// a shape resolveAmount evaluates (a bare Count head outside the Valid
// family) is skipped entirely -- the host's own printed "*/*" base stays
// unresolvable, not silently zero.
func TestApplyContinuousCharacteristicDefiningSkipsUnresolvableSVar(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p, other := g.Players()[0], g.Players()[1]
	g.Player(p).Life, g.Player(other).Life = 20, 20
	reg, err := cardtype.LoadRegistry(strings.NewReader("[CreatureTypes]\nElf\n"))
	if err != nil {
		t.Fatalf("LoadRegistry: %v", err)
	}
	raw := &carddb.Card{Filename: "Test CDA Unresolvable"}
	raw.Faces[0].Present = true
	raw.Faces[0].Name = "Test CDA Unresolvable"
	raw.Faces[0].Type = cardtype.Parse(reg, "Creature Elf")
	raw.Faces[0].Power, raw.Faces[0].Toughness = "*", "*"
	raw.Faces[0].Statics = []string{"Mode$ Continuous | CharacteristicDefining$ True | SetPower$ X | SetToughness$ X"}
	raw.Faces[0].SVars.Set("X", "Count$CardPower")
	def, err := compile.Compile(raw)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	cda := g.NewCard(def, p, engine.Battlefield)

	engine.CheckStateBasedActions(g, engine.NewScriptedController())

	if _, ok := g.Card(cda).Power(); ok {
		t.Error("Power() resolved, want unresolvable -- Count$CardPower is outside the Valid family this port evaluates")
	}
}

// TestApplyContinuousCharacteristicDefiningSkipsDistinctPropertyCount proves
// a Valid family argument itself carrying a `$`-suffixed distinct-value
// operator (Tarmogoyf's own real `Count$ValidGraveyard Card$CardTypes`) is
// skipped, not silently resolved as a plain match count against `Card`
// (which would wrongly compute 0 every time, since every card matches
// `Card` -- expr.Count.DistinctProperty's own doc comment; this is a
// regression test for a real bug caught after the fact, not a hypothetical
// one).
func TestApplyContinuousCharacteristicDefiningSkipsDistinctPropertyCount(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p, other := g.Players()[0], g.Players()[1]
	g.Player(p).Life, g.Player(other).Life = 20, 20
	reg, err := cardtype.LoadRegistry(strings.NewReader("[CreatureTypes]\nLhurgoyf\n"))
	if err != nil {
		t.Fatalf("LoadRegistry: %v", err)
	}
	raw := &carddb.Card{Filename: "Test Tarmogoyf"}
	raw.Faces[0].Present = true
	raw.Faces[0].Name = "Test Tarmogoyf"
	raw.Faces[0].Type = cardtype.Parse(reg, "Creature Lhurgoyf")
	raw.Faces[0].Power, raw.Faces[0].Toughness = "*", "*"
	raw.Faces[0].Statics = []string{"Mode$ Continuous | CharacteristicDefining$ True | SetPower$ X | SetToughness$ X"}
	raw.Faces[0].SVars.Set("X", "Count$ValidGraveyard Card$CardTypes")
	def, err := compile.Compile(raw)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	goyf := g.NewCard(def, p, engine.Battlefield)
	g.NewCard(creatureDefPT(t, "1", "1"), p, engine.Graveyard)

	engine.CheckStateBasedActions(g, engine.NewScriptedController())

	if pw, ok := g.Card(goyf).Power(); ok {
		t.Errorf("Power() = (%d, true), want unresolvable -- Card$CardTypes is a distinct-value count this port does not evaluate, and must not silently resolve to a plain match count against \"Card\"", pw)
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

// TestApplyContinuousColorAddsColorToMatchingCreatures proves Layer 5's own
// AddColor$: a blanket "creatures you control are also blue" effect unions
// Blue in without displacing the creature's own printed Red.
func TestApplyContinuousColorAddsColorToMatchingCreatures(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p, other := g.Players()[0], g.Players()[1]
	g.Player(p).Life, g.Player(other).Life = 20, 20
	g.NewCard(continuousDef(t, "Test Color Anthem", "Mode$ Continuous | Affected$ Creature.YouCtrl | AddColor$ Blue"), p, engine.Battlefield)
	creature := g.NewCard(creatureDefManaCost(t, "R"), p, engine.Battlefield)

	engine.CheckStateBasedActions(g, engine.NewScriptedController())

	colors := g.Card(creature).Colors()
	if !colors.Has(mana.Blue) {
		t.Errorf("Colors() = %v, want it to carry the added Blue", colors)
	}
	if !colors.Has(mana.Red) {
		t.Errorf("Colors() = %v, want it to still carry its own printed Red", colors)
	}
}

// TestApplyContinuousColorSetColorOverwrites proves SetColor$'s own
// "replace outright" semantics (Java's overwriteColors): the creature's own
// printed Red is gone, not merely joined by Black.
func TestApplyContinuousColorSetColorOverwrites(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p, other := g.Players()[0], g.Players()[1]
	g.Player(p).Life, g.Player(other).Life = 20, 20
	g.NewCard(continuousDef(t, "Test Color Setter", "Mode$ Continuous | Affected$ Creature.YouCtrl | SetColor$ Black"), p, engine.Battlefield)
	creature := g.NewCard(creatureDefManaCost(t, "R"), p, engine.Battlefield)

	engine.CheckStateBasedActions(g, engine.NewScriptedController())

	colors := g.Card(creature).Colors()
	if !colors.Has(mana.Black) {
		t.Errorf("Colors() = %v, want Black", colors)
	}
	if colors.Has(mana.Red) {
		t.Errorf("Colors() = %v, want the printed Red replaced, not joined", colors)
	}
}

// TestApplyContinuousColorDoesNotAffectNonMatchingCreatures mirrors the
// Layer 7/Layer 4 non-matching tests: an opponent's creature is untouched.
func TestApplyContinuousColorDoesNotAffectNonMatchingCreatures(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p, other := g.Players()[0], g.Players()[1]
	g.Player(p).Life, g.Player(other).Life = 20, 20
	g.NewCard(continuousDef(t, "Test Color Anthem", "Mode$ Continuous | Affected$ Creature.YouCtrl | AddColor$ Blue"), p, engine.Battlefield)
	theirs := g.NewCard(creatureDefManaCost(t, "R"), other, engine.Battlefield)

	engine.CheckStateBasedActions(g, engine.NewScriptedController())

	if colors := g.Card(theirs).Colors(); colors.Has(mana.Blue) {
		t.Errorf("Colors() = %v, an opponent's anthem must not add Blue to it", colors)
	}
}

// TestApplyContinuousColorRecomputesWhenSourceLeaves mirrors the Layer
// 7/Layer 4 leave tests: once the color-granting source itself leaves the
// battlefield, the added color is gone on the very next check.
func TestApplyContinuousColorRecomputesWhenSourceLeaves(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p, other := g.Players()[0], g.Players()[1]
	g.Player(p).Life, g.Player(other).Life = 20, 20
	anthem := g.NewCard(continuousDef(t, "Test Color Anthem", "Mode$ Continuous | Affected$ Creature.YouCtrl | AddColor$ Blue"), p, engine.Battlefield)
	creature := g.NewCard(creatureDefManaCost(t, "R"), p, engine.Battlefield)

	engine.CheckStateBasedActions(g, engine.NewScriptedController())
	if colors := g.Card(creature).Colors(); !colors.Has(mana.Blue) {
		t.Fatalf("setup: Colors() = %v, want it to carry Blue", colors)
	}

	g.Move(anthem, engine.Graveyard, p)
	engine.CheckStateBasedActions(g, engine.NewScriptedController())

	if colors := g.Card(creature).Colors(); colors.Has(mana.Blue) {
		t.Errorf("Colors() after the anthem left = %v, want Blue gone", colors)
	}
}

// TestApplyContinuousColorSetColorAll proves the fixed "All" token Java's
// own getColorsFromParam special-cases: SetColor$ All resolves to every
// color (mana.AllColors), not a literal color named "All".
func TestApplyContinuousColorSetColorAll(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p, other := g.Players()[0], g.Players()[1]
	g.Player(p).Life, g.Player(other).Life = 20, 20
	g.NewCard(continuousDef(t, "Test All Colors", "Mode$ Continuous | Affected$ Creature.YouCtrl | SetColor$ All"), p, engine.Battlefield)
	creature := g.NewCard(creatureDefManaCost(t, "R"), p, engine.Battlefield)

	engine.CheckStateBasedActions(g, engine.NewScriptedController())

	if got := g.Card(creature).Colors(); got != mana.AllColors {
		t.Errorf("Colors() = %v, want mana.AllColors", got)
	}
}

// TestApplyContinuousColorSetColorColorless proves the other fixed token:
// SetColor$ Colorless resolves to no color at all, replacing the creature's
// own printed Red rather than leaving it untouched.
func TestApplyContinuousColorSetColorColorless(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p, other := g.Players()[0], g.Players()[1]
	g.Player(p).Life, g.Player(other).Life = 20, 20
	g.NewCard(continuousDef(t, "Test Colorless", "Mode$ Continuous | Affected$ Creature.YouCtrl | SetColor$ Colorless"), p, engine.Battlefield)
	creature := g.NewCard(creatureDefManaCost(t, "R"), p, engine.Battlefield)

	engine.CheckStateBasedActions(g, engine.NewScriptedController())

	if got := g.Card(creature).Colors(); got != 0 {
		t.Errorf("Colors() = %v, want colorless (0)", got)
	}
}

// TestApplyContinuousColorSkipsChosenColor proves an AddColor$/SetColor$
// token this port cannot resolve at runtime (ChosenColor) skips the whole
// line rather than crashing or resolving to no color.
func TestApplyContinuousColorSkipsChosenColor(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p, other := g.Players()[0], g.Players()[1]
	g.Player(p).Life, g.Player(other).Life = 20, 20
	g.NewCard(continuousDef(t, "Test Chosen Color Anthem", "Mode$ Continuous | Affected$ Creature.YouCtrl | AddColor$ ChosenColor"), p, engine.Battlefield)
	creature := g.NewCard(creatureDefManaCost(t, "R"), p, engine.Battlefield)

	engine.CheckStateBasedActions(g, engine.NewScriptedController())

	if colors := g.Card(creature).Colors(); colors != mana.Red {
		t.Errorf("Colors() = %v, want unchanged Red -- an unresolvable ChosenColor token must not apply", colors)
	}
}

// TestApplyContinuousKeywordGrantsKeywordToMatchingCreatures proves Layer
// 6's own AddKeyword$: a blanket "creatures you control have flying" effect
// grants it, read back through HasKeyword the same way a printed keyword
// already is.
func TestApplyContinuousKeywordGrantsKeywordToMatchingCreatures(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p, other := g.Players()[0], g.Players()[1]
	g.Player(p).Life, g.Player(other).Life = 20, 20
	g.NewCard(continuousDef(t, "Test Keyword Anthem", "Mode$ Continuous | Affected$ Creature.YouCtrl | AddKeyword$ Flying"), p, engine.Battlefield)
	creature := g.NewCard(creatureDefPT(t, "2", "2"), p, engine.Battlefield)

	engine.CheckStateBasedActions(g, engine.NewScriptedController())

	if !g.Card(creature).HasKeyword("Flying") {
		t.Error("HasKeyword(\"Flying\") = false, want true -- the anthem's own AddKeyword$ should have granted it")
	}
}

// TestApplyContinuousKeywordGrantsBothTokens proves the " & "-separated
// multi-keyword shape (5 of 256 real AddKeyword$ lines carry more than one
// core-type token for AddType$; AddKeyword$'s own corpus carries the
// identical separator, "Flying & Haste" among the real samples) grants
// every token, not just the first.
func TestApplyContinuousKeywordGrantsBothTokens(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p, other := g.Players()[0], g.Players()[1]
	g.Player(p).Life, g.Player(other).Life = 20, 20
	g.NewCard(continuousDef(t, "Test Keyword Anthem", "Mode$ Continuous | Affected$ Creature.YouCtrl | AddKeyword$ Flying & Haste"), p, engine.Battlefield)
	creature := g.NewCard(creatureDefPT(t, "2", "2"), p, engine.Battlefield)

	engine.CheckStateBasedActions(g, engine.NewScriptedController())

	c := g.Card(creature)
	if !c.HasKeyword("Flying") || !c.HasKeyword("Haste") {
		t.Errorf("HasKeyword: Flying = %v, Haste = %v, want true and true", c.HasKeyword("Flying"), c.HasKeyword("Haste"))
	}
}

// TestApplyContinuousKeywordDoesNotAffectNonMatchingCreatures mirrors the
// Layer 7/4/5 non-matching tests: an opponent's creature is untouched by a
// "creatures you control" keyword-granting anthem.
func TestApplyContinuousKeywordDoesNotAffectNonMatchingCreatures(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p, other := g.Players()[0], g.Players()[1]
	g.Player(p).Life, g.Player(other).Life = 20, 20
	g.NewCard(continuousDef(t, "Test Keyword Anthem", "Mode$ Continuous | Affected$ Creature.YouCtrl | AddKeyword$ Flying"), p, engine.Battlefield)
	theirs := g.NewCard(creatureDefPT(t, "2", "2"), other, engine.Battlefield)

	engine.CheckStateBasedActions(g, engine.NewScriptedController())

	if g.Card(theirs).HasKeyword("Flying") {
		t.Error("HasKeyword(\"Flying\") = true, an opponent's anthem must not grant it")
	}
}

// TestApplyContinuousKeywordRecomputesWhenSourceLeaves mirrors the Layer
// 7/4/5 leave tests: once the keyword-granting source itself leaves the
// battlefield, the granted keyword is gone on the very next check.
func TestApplyContinuousKeywordRecomputesWhenSourceLeaves(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p, other := g.Players()[0], g.Players()[1]
	g.Player(p).Life, g.Player(other).Life = 20, 20
	anthem := g.NewCard(continuousDef(t, "Test Keyword Anthem", "Mode$ Continuous | Affected$ Creature.YouCtrl | AddKeyword$ Flying"), p, engine.Battlefield)
	creature := g.NewCard(creatureDefPT(t, "2", "2"), p, engine.Battlefield)

	engine.CheckStateBasedActions(g, engine.NewScriptedController())
	if !g.Card(creature).HasKeyword("Flying") {
		t.Fatal("setup: HasKeyword(\"Flying\") = false, want true")
	}

	g.Move(anthem, engine.Graveyard, p)
	engine.CheckStateBasedActions(g, engine.NewScriptedController())

	if g.Card(creature).HasKeyword("Flying") {
		t.Error("HasKeyword(\"Flying\") after the anthem left = true, want false")
	}
}

// TestApplyContinuousKeywordSkipsDynamicValue proves an AddKeyword$ token
// this port cannot resolve at runtime (a ChosenColor-qualified Protection
// grant) skips the whole line rather than granting a literal, wrong keyword.
func TestApplyContinuousKeywordSkipsDynamicValue(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p, other := g.Players()[0], g.Players()[1]
	g.Player(p).Life, g.Player(other).Life = 20, 20
	g.NewCard(continuousDef(t, "Test Chosen Protection", "Mode$ Continuous | Affected$ Creature.YouCtrl | AddKeyword$ Protection:Card.ChosenColor:chosenColor"), p, engine.Battlefield)
	creature := g.NewCard(creatureDefPT(t, "2", "2"), p, engine.Battlefield)

	engine.CheckStateBasedActions(g, engine.NewScriptedController())

	if g.Card(creature).HasKeyword("Protection") {
		t.Error("HasKeyword(\"Protection\") = true, an unresolvable ChosenColor token must not apply")
	}
}

// TestApplyContinuousKeywordSkipsRemoveKeywordCombo proves a line pairing
// AddKeyword$ with RemoveKeyword$ (a real "gains X, loses Y" shape) is
// skipped whole: applying AddKeyword$ alone would leave the creature with
// both the old and new keyword, an actively wrong answer this port refuses
// to give rather than shipping half of a line.
func TestApplyContinuousKeywordSkipsRemoveKeywordCombo(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p, other := g.Players()[0], g.Players()[1]
	g.Player(p).Life, g.Player(other).Life = 20, 20
	g.NewCard(continuousDef(t, "Test Swap Anthem", "Mode$ Continuous | Affected$ Creature.YouCtrl | AddKeyword$ Reach | RemoveKeyword$ Flying"), p, engine.Battlefield)
	creature := g.NewCard(creatureDefPTKeywords(t, "2", "2", "Flying"), p, engine.Battlefield)

	engine.CheckStateBasedActions(g, engine.NewScriptedController())

	c := g.Card(creature)
	if c.HasKeyword("Reach") {
		t.Error("HasKeyword(\"Reach\") = true, want the whole line skipped (RemoveKeyword$ is not evaluated), not just partially applied")
	}
	if !c.HasKeyword("Flying") {
		t.Error("HasKeyword(\"Flying\") = false, want the creature's own printed Flying left untouched by the skipped line")
	}
}

// TestApplyContinuousKeywordGrantedFlyingAffectsCanBlock proves the fold
// reaches further than HasKeyword alone: a continuously-granted Flying
// makes a creature unblockable by a grounded creature the identical way a
// printed Flying keyword already does (cantBlockByKeywords, staticability.go)
// -- Layer 6's own first real caller reaching all the way into block
// legality, not just a bare HasKeyword check.
func TestApplyContinuousKeywordGrantedFlyingAffectsCanBlock(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	a, b := g.Players()[0], g.Players()[1]
	g.Player(a).Life, g.Player(b).Life = 20, 20
	g.NewCard(continuousDef(t, "Test Flying Anthem", "Mode$ Continuous | Affected$ Creature.YouCtrl | AddKeyword$ Flying"), a, engine.Battlefield)
	attacker := g.NewCard(creatureDefPT(t, "2", "2"), a, engine.Battlefield)
	ground := g.NewCard(creatureDefPT(t, "2", "2"), b, engine.Battlefield)

	engine.CheckStateBasedActions(g, engine.NewScriptedController())

	if g.CanBlock(attacker, ground) {
		t.Error("CanBlock(continuously-flying attacker, grounded blocker) = true, want false")
	}
}
