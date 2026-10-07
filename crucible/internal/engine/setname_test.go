package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/carddb/compile"
	"github.com/jczastkiewicz/crucible/internal/cardtype"
	"github.com/jczastkiewicz/crucible/internal/engine"
	"github.com/jczastkiewicz/crucible/pkg/javarand"
)

const setNameKitStatic = "Mode$ Continuous | AffectedDefined$ Equipped | Affected$ Creature | SetName$ Test Legend Two"

// gameWithCreatureNames is a game whose database holds one non-legendary and
// one legendary creature, for the legend rule's name lookup.
func gameWithCreatureNames(t *testing.T) *engine.Game {
	t.Helper()
	reg := attachmentTypeRegistry(t)
	plain := &compile.Card{Name: "Plain Bear"}
	plain.Faces[0].Name = "Plain Bear"
	plain.Faces[0].Type = cardtype.Parse(reg, "Creature Elf")
	legend := &compile.Card{Name: "Legend Bear"}
	legend.Faces[0].Name = "Legend Bear"
	legend.Faces[0].Type = cardtype.Parse(reg, "Legendary Creature Elf")
	noncreature := &compile.Card{Name: "Plain Rock"}
	noncreature.Faces[0].Name = "Plain Rock"
	noncreature.Faces[0].Type = cardtype.Parse(reg, "Artifact")
	db := compile.NewDB(map[string]*compile.Card{"Plain Bear": plain, "Legend Bear": legend, "Plain Rock": noncreature})
	g := engine.NewGame(db, javarand.New(1), []string{"a", "b"})
	g.Player(g.Players()[0]).Life, g.Player(g.Players()[1]).Life = 20, 20
	return g
}

// StaticAbilityContinuous.java:647-655: SetName$ renames the affected permanent
// (Card.getName reads the last overwrite); the name ends with the effect.
func TestSetNameRenamesTheEquippedCreature(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p := g.Players()[0]
	creature := g.NewCard(legendaryCreatureDef(t, "Test Legend One"), p, engine.Battlefield)
	kit := g.NewCard(equipmentDefWithStatic(t, "Test Rename Kit", setNameKitStatic), p, engine.Battlefield)
	g.Attach(kit, creature)
	sba(g)
	if got := g.Card(creature).Name(); got != "Test Legend Two" {
		t.Errorf("name = %q, want Test Legend Two", got)
	}
	g.Unattach(kit)
	sba(g)
	if got := g.Card(creature).Name(); got != "Test Legend One" {
		t.Errorf("name after unattaching = %q, want the printed name back", got)
	}
}

// Psychic Paper: SetName$ ChosenName is the host's last NameCard pick, and no
// pick writes nothing.
func TestSetNameChosenNameReadsTheHostsLastPick(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p := g.Players()[0]
	creature := g.NewCard(legendaryCreatureDef(t, "Test Legend One"), p, engine.Battlefield)
	kit := g.NewCard(equipmentDefWithStatic(t, "Test Paper",
		"Mode$ Continuous | AffectedDefined$ Equipped | Affected$ Creature | SetName$ ChosenName"), p, engine.Battlefield)
	g.Attach(kit, creature)
	sba(g)
	if got := g.Card(creature).Name(); got != "Test Legend One" {
		t.Errorf("name with no pick = %q, want the printed name", got)
	}
	g.Card(kit).Memory.AddNamedCard("First Pick")
	g.Card(kit).Memory.AddNamedCard("Second Pick")
	sba(g)
	if got := g.Card(creature).Name(); got != "Second Pick" {
		t.Errorf("name = %q, want the last pick", got)
	}
}

// A renamed legend clashes with a permanent bearing that name (CR 704.5j
// groups by Card.getName).
func TestSetNameMakesTheLegendRuleSeeTheNewName(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p := g.Players()[0]
	first := g.NewCard(legendaryCreatureDef(t, "Test Legend One"), p, engine.Battlefield)
	second := g.NewCard(legendaryCreatureDef(t, "Test Legend Two"), p, engine.Battlefield)
	kit := g.NewCard(equipmentDefWithStatic(t, "Test Rename Kit", setNameKitStatic), p, engine.Battlefield)
	g.Attach(kit, first)

	c := engine.NewScriptedController()
	c.QueueLegendaryToKeep(second)
	engine.CheckStateBasedActions(g, c)
	if z := g.Card(first).Zone; z != engine.Graveyard {
		t.Errorf("renamed legend zone = %v, want Graveyard", z)
	}
	if z := g.Card(second).Zone; z != engine.Battlefield {
		t.Errorf("kept legend zone = %v, want Battlefield", z)
	}
}

// Corner Case 1 (GameAction.java:2035-2037): a legendary named like a plain
// creature card clashes with a Spy Kit wearer, since the wearer has that name.
func TestLegendRuleCornerCaseOneNonLegendaryCreatureName(t *testing.T) {
	t.Parallel()

	g := gameWithCreatureNames(t)
	p := g.Players()[0]
	named := g.NewCard(legendaryCreatureDef(t, "Plain Bear"), p, engine.Battlefield)
	wearer := g.NewCard(legendaryCreatureDef(t, "Test Legend Two"), p, engine.Battlefield)
	kit := g.NewCard(equipmentDefWithStatic(t, "Test Spy Kit", spyKitStatic), p, engine.Battlefield)
	g.Attach(kit, wearer)

	c := engine.NewScriptedController()
	c.QueueLegendaryToKeep(named)
	engine.CheckStateBasedActions(g, c)
	if z := g.Card(wearer).Zone; z != engine.Graveyard {
		t.Errorf("Spy Kit wearer zone = %v, want Graveyard", z)
	}
	if z := g.Card(named).Zone; z != engine.Battlefield {
		t.Errorf("kept legend zone = %v, want Battlefield", z)
	}
}

// The lookup is a non-legendary creature face: a legendary creature's name or
// a noncreature's does not pull the wearer in, and a game without that name
// (or with no database) never asks (an empty queue would panic).
func TestLegendRuleCornerCaseOneNeedsANonLegendaryCreatureName(t *testing.T) {
	t.Parallel()

	for _, name := range []string{"Legend Bear", "Plain Rock", "Unknown Name"} {
		g := gameWithCreatureNames(t)
		p := g.Players()[0]
		named := g.NewCard(legendaryCreatureDef(t, name), p, engine.Battlefield)
		wearer := g.NewCard(legendaryCreatureDef(t, "Test Legend Two"), p, engine.Battlefield)
		kit := g.NewCard(equipmentDefWithStatic(t, "Test Spy Kit", spyKitStatic), p, engine.Battlefield)
		g.Attach(kit, wearer)

		engine.CheckStateBasedActions(g, engine.NewScriptedController())
		if g.Card(named).Zone != engine.Battlefield || g.Card(wearer).Zone != engine.Battlefield {
			t.Errorf("%s: a legend left the battlefield, want both to stay", name)
		}
	}

	g := newGame(t, "a", "b")
	p := g.Players()[0]
	named := g.NewCard(legendaryCreatureDef(t, "Plain Bear"), p, engine.Battlefield)
	wearer := g.NewCard(legendaryCreatureDef(t, "Test Legend Two"), p, engine.Battlefield)
	kit := g.NewCard(equipmentDefWithStatic(t, "Test Spy Kit", spyKitStatic), p, engine.Battlefield)
	g.Attach(kit, wearer)
	engine.CheckStateBasedActions(g, engine.NewScriptedController())
	if g.Card(named).Zone != engine.Battlefield || g.Card(wearer).Zone != engine.Battlefield {
		t.Error("no database: a legend left the battlefield")
	}
}
