package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/carddb"
	"github.com/jczastkiewicz/crucible/internal/carddb/compile"
	"github.com/jczastkiewicz/crucible/internal/cardtype"
	"github.com/jczastkiewicz/crucible/internal/engine"
	"github.com/jczastkiewicz/crucible/internal/mana"
)

// powerDoublerDef is an instant that gives its target creature +X/+0, X being
// the target's own power (Targeted$CardPower).
func powerDoublerDef(t *testing.T) *compile.Card {
	t.Helper()

	raw := &carddb.Card{Filename: "power-doubler"}
	raw.Faces[0].Present = true
	raw.Faces[0].Name = "Power Doubler"
	raw.Faces[0].Type = cardtype.Parse(attachmentTypeRegistry(t), "Instant")
	raw.Faces[0].ManaCost = mana.MustParse("G")
	raw.Faces[0].Abilities = []string{"SP$ Pump | ValidTgts$ Creature | NumAtt$ +X"}
	raw.Faces[0].SVars.Set("X", "Targeted$CardPower")
	c, err := compile.Compile(raw)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	return c
}

// lifeWatcherDef is a permanent that gains its controller life equal to the
// power of each other creature that enters under their control
// (TriggeredCard$CardPower).
func lifeWatcherDef(t *testing.T) *compile.Card {
	t.Helper()

	raw := &carddb.Card{Filename: "life-watcher"}
	raw.Faces[0].Present = true
	raw.Faces[0].Name = "Life Watcher"
	raw.Faces[0].Type = cardtype.Parse(attachmentTypeRegistry(t), "Enchantment")
	raw.Faces[0].Triggers = []string{"Mode$ ChangesZone | Destination$ Battlefield | ValidCard$ Creature.YouCtrl | TriggerZones$ Battlefield | Execute$ Gain"}
	raw.Faces[0].SVars.Set("Gain", "DB$ GainLife | Defined$ You | LifeAmount$ X")
	raw.Faces[0].SVars.Set("X", "TriggeredCard$CardPower")
	c, err := compile.Compile(raw)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	return c
}

// An amount that names the ability's targets (AbilityUtils.calculateAmount's
// Targeted branch) reads the chosen creature.
func TestTargetedAmountIsTheTargetsPower(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGameOn(t, scenarioDB(t))
	g.SetTurnState(1, p, engine.Main1)
	bears := g.NewCard(corpusCard(t, "Grizzly Bears"), p, engine.Battlefield)
	spell := g.NewCard(powerDoublerDef(t), p, engine.Hand)
	g.Player(p).ManaPool.Add(mana.Green, 1)
	c := engine.NewScriptedController()
	c.QueueTargets([]engine.EntityID{engine.CardEntity(bears)})
	castThenResolve(t, g, p, spell, c)
	if got := powerOf(t, g, bears); got != 4 {
		t.Errorf("Bears power = %d, want 4 (+2 from its own power)", got)
	}
}

// TriggeredCard$ in an enters trigger is the creature that entered.
func TestTriggeredCardAmountIsTheEnteringCreature(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGameOn(t, scenarioDB(t))
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life = 10
	g.NewCard(lifeWatcherDef(t), p, engine.Battlefield)
	bears := g.NewCard(corpusCard(t, "Grizzly Bears"), p, engine.Hand)
	g.Player(p).ManaPool.Add(mana.Green, 2)
	c := engine.NewScriptedController()
	queueXPayGeneric(c, mana.ShardG, 1)
	castThenResolve(t, g, p, bears, c)
	if got := g.Player(p).Life; got != 12 {
		t.Errorf("life = %d, want 12: the Bears' power", got)
	}
}

// TriggeredCard$ in a dies trigger is read from last-known information
// (Bottle Golems: "you gain life equal to its power").
func TestTriggeredCardAmountOfADyingCreatureIsItsLastKnownPower(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGameOn(t, scenarioDB(t))
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life = 10
	golem := g.NewCard(corpusCard(t, "Bottle Golems"), p, engine.Battlefield)
	power := powerOf(t, g, golem)
	g.Card(golem).Damage.Mark(20, false)
	c := engine.NewScriptedController()
	engine.CheckStateBasedActions(g, c)
	if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if got, want := g.Player(p).Life, 10+power; got != want {
		t.Errorf("life = %d, want %d: the Golems' power as it died", got, want)
	}
}
