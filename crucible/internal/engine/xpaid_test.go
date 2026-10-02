package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/carddb"
	"github.com/jczastkiewicz/crucible/internal/carddb/compile"
	"github.com/jczastkiewicz/crucible/internal/cardtype"
	"github.com/jczastkiewicz/crucible/internal/engine"
	"github.com/jczastkiewicz/crucible/internal/mana"
)

// xCounterCreatureDef is a 1/1 creature that enters with X +1/+1 counters,
// X being the X it was cast with.
func xCounterCreatureDef(t *testing.T) *compile.Card {
	t.Helper()

	raw := &carddb.Card{Filename: "x-counter-creature"}
	raw.Faces[0].Present = true
	raw.Faces[0].Name = "X Counter Creature"
	raw.Faces[0].Type = cardtype.Parse(attachmentTypeRegistry(t), "Creature Elf")
	raw.Faces[0].Power, raw.Faces[0].Toughness = "1", "1"
	raw.Faces[0].ManaCost = mana.MustParse("X")
	raw.Faces[0].Keywords = []string{"etbCounter:P1P1:X"}
	raw.Faces[0].SVars.Set("X", "Count$xPaid")
	c, err := compile.Compile(raw)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	return c
}

// xPutOntoBattlefieldDef is a sorcery with {X} in its cost that puts a creature
// card from its controller's hand onto the battlefield.
func xPutOntoBattlefieldDef(t *testing.T) *compile.Card {
	t.Helper()

	raw := &carddb.Card{Filename: "x-wave"}
	raw.Faces[0].Present = true
	raw.Faces[0].Name = "X Wave"
	raw.Faces[0].Type = cardtype.Parse(attachmentTypeRegistry(t), "Sorcery")
	raw.Faces[0].ManaCost = mana.MustParse("X")
	raw.Faces[0].Abilities = []string{"SP$ ChangeZone | Hidden$ True | Origin$ Hand | Destination$ Battlefield | ChangeType$ Creature | ChangeNum$ 1"}
	c, err := compile.Compile(raw)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	return c
}

// Count$xPaid (AbilityUtils.java:1631): an ability reads the X its caster
// announced. Blaze deals X damage to any target.
func TestXPaidIsTheAnnouncedX(t *testing.T) {
	t.Parallel()

	g, p, other := newTwoPlayerGameOn(t, scenarioDB(t))
	g.SetTurnState(1, p, engine.Main1)
	g.Player(other).Life = 20
	blaze := g.NewCard(corpusCard(t, "Blaze"), p, engine.Hand)
	g.Player(p).ManaPool.Add(mana.Red, 5)
	c := engine.NewScriptedController()
	c.QueuePayX(4)
	queueXPayGeneric(c, mana.ShardR, 4)
	c.QueueTargets([]engine.EntityID{engine.PlayerEntity(other)})
	if !g.CastSpell(p, blaze, c) {
		t.Fatal("cast failed")
	}
	if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if got := g.Player(other).Life; got != 16 {
		t.Errorf("life = %d, want 16 after X=4 damage", got)
	}
}

// Count$xPaid is the resolving ability's own X: a card that an X spell puts
// onto the battlefield reads its own cast X (0: it was not cast), not the
// spell's.
func TestXPaidOfAnEnteringCardIsNotTheResolvingSpellsX(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGameOn(t, scenarioDB(t))
	g.SetTurnState(1, p, engine.Main1)
	wave := g.NewCard(xPutOntoBattlefieldDef(t), p, engine.Hand)
	ballista := g.NewCard(xCounterCreatureDef(t), p, engine.Hand)
	g.Player(p).ManaPool.Add(mana.Green, 3)
	c := engine.NewScriptedController()
	c.QueuePayX(3)
	queueXPayGeneric(c, mana.ShardG, 3)
	c.QueueCardChoice([]engine.CardID{ballista})
	castThenResolve(t, g, p, wave, c)
	if got := g.Card(ballista).Zone; got != engine.Battlefield {
		t.Fatalf("the creature is in %v, want Battlefield", got)
	}
	if got := g.Card(ballista).Counters.Count(engine.P1P1); got != 0 {
		t.Errorf("a creature put onto the battlefield with X=3 around it has %d counters, want 0", got)
	}
}

// CR 601.2b announces X before the targets of CR 601.2c, so a restriction on
// them can name it: Repeal's cmcEQX sees the X the caster chose, not 0.
func TestTargetRestrictionOnXSeesTheAnnouncedX(t *testing.T) {
	t.Parallel()

	g, p, other := newTwoPlayerGameOn(t, scenarioDB(t))
	g.SetTurnState(1, p, engine.Main1)
	zero := g.NewCard(creatureDefPT(t, "1", "1"), other, engine.Battlefield)
	two := g.NewCard(creatureDefManaCost(t, "2"), other, engine.Battlefield)
	repeal := g.NewCard(corpusCard(t, "Repeal"), p, engine.Hand)
	g.Player(p).ManaPool.Add(mana.Blue, 3)
	c := engine.NewScriptedController()
	c.QueuePayX(2)
	queueXPayGeneric(c, mana.ShardU, 2)
	c.QueueTargets([]engine.EntityID{engine.CardEntity(two)})
	if !g.CastSpell(p, repeal, c) {
		t.Fatal("cast refused")
	}
	if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if got := g.Card(two).Zone; got != engine.Hand {
		t.Errorf("the mana value 2 creature is in %v, want Hand", got)
	}
	if got := g.Card(zero).Zone; got != engine.Battlefield {
		t.Errorf("the mana value 0 creature is in %v, want Battlefield", got)
	}
}
