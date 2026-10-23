package engine_test

import (
	"github.com/jczastkiewicz/crucible/internal/mana"
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
)

// Untap.doUntap (Untap.java:104-157): UntapAdjust caps how many permanents of
// a type untap and the player picks which; a Mode$ UntapOtherPlayer static
// untaps its ValidCard$ permanents in the other player's step too.

func tappedCreatures(t *testing.T, g *engine.Game, p engine.PlayerID, n int) []engine.CardID {
	t.Helper()
	ids := make([]engine.CardID, n)
	for i := range ids {
		ids[i] = g.NewCard(creatureDefPT(t, "1", "1"), p, engine.Battlefield)
		g.Card(ids[i]).Tapped = true
	}
	return ids
}

func TestUntapAdjustLetsPlayerPickWhichCreatureUntaps(t *testing.T) {
	t.Parallel()

	g, p, other := newTwoPlayerGame(t)
	g.NewCard(scriptDef(t, "Test Smoke", "Enchantment",
		"S:Mode$ Continuous | Affected$ Player | AddKeyword$ UntapAdjust:Creature:1"), other, engine.Battlefield)
	creatures := tappedCreatures(t, g, p, 3)
	engine.CheckStateBasedActions(g, engine.NewScriptedController())
	c := engine.NewScriptedController()
	c.QueueCardChoice([]engine.CardID{creatures[1]})

	g.StartTurn(p, c)

	for i, id := range creatures {
		if got, want := g.Card(id).Tapped, i != 1; got != want {
			t.Errorf("creature %d tapped = %v, want %v", i, got, want)
		}
	}
}

func TestUntapOtherPlayerUntapsInAnotherPlayersStep(t *testing.T) {
	t.Parallel()

	g, p, other := newTwoPlayerGame(t)
	g.NewCard(scriptDef(t, "Test Seedborn", "Enchantment",
		"S:Mode$ UntapOtherPlayer | ValidCard$ Creature.YouCtrl"), other, engine.Battlefield)
	mine := tappedCreatures(t, g, p, 1)
	theirs := tappedCreatures(t, g, other, 1)

	g.StartTurn(p, engine.NewScriptedController())

	if g.Card(theirs[0]).Tapped {
		t.Error("the static's creature stayed tapped in the other player's untap step")
	}
	if g.Card(mine[0]).Tapped {
		t.Error("the active player's own creature stayed tapped")
	}
}

func TestOnlyUntapChosenUntapsOnlyThePickedType(t *testing.T) {
	t.Parallel()

	g, p, other := newTwoPlayerGame(t)
	g.NewCard(scriptDef(t, "Test Winter Orb", "Artifact",
		"S:Mode$ Continuous | Affected$ Player | AddKeyword$ OnlyUntapChosen:Artifact,Creature,Land"), other, engine.Battlefield)
	creature := tappedCreatures(t, g, p, 1)[0]
	rock := g.NewCard(scriptDef(t, "Test Rock", "Artifact"), p, engine.Battlefield)
	g.Card(rock).Tapped = true
	engine.CheckStateBasedActions(g, engine.NewScriptedController())
	c := engine.NewScriptedController()
	c.QueueOptionNamed("Creature")

	g.StartTurn(p, c)

	if g.Card(creature).Tapped || !g.Card(rock).Tapped {
		t.Errorf("creature tapped = %v, artifact tapped = %v; want only the creature untapped", g.Card(creature).Tapped, g.Card(rock).Tapped)
	}
}

// Mode$ Crewed (MagicStack.java:400-408) fires once per creature tapped for a
// Vehicle's Crew ability, with ValidCrew$ naming the crewer: a trigger on the
// crewer fires, one on a creature that did not crew does not.
func TestCrewedTriggerFiresForTheCrewer(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGame(t)
	vehicle := g.NewCard(scriptDef(t, "Test Vehicle", "Artifact Vehicle", "K:Crew:1"), p, engine.Battlefield)
	crewer := g.NewCard(scriptDef(t, "Test Crewer", "Creature Elf",
		"T:Mode$ Crewed | ValidCrew$ Card.Self | Execute$ TrigGain | TriggerZones$ Battlefield",
		"SVar:TrigGain:DB$ GainLife | Defined$ You | LifeAmount$ 3"), p, engine.Battlefield)
	g.NewCard(scriptDef(t, "Test Bystander", "Creature Elf",
		"T:Mode$ Crewed | ValidCrew$ Card.Self | Execute$ TrigGain | TriggerZones$ Battlefield",
		"SVar:TrigGain:DB$ GainLife | Defined$ You | LifeAmount$ 100"), p, engine.Battlefield)
	engine.CheckStateBasedActions(g, engine.NewScriptedController())
	before := g.Player(p).Life
	c := engine.NewScriptedController()
	c.QueueCardChoice([]engine.CardID{crewer})

	if !g.ActivateAbility(p, vehicle, 0, c) {
		t.Fatal("Crew could not be activated")
	}
	if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}

	if got := g.Player(p).Life - before; got != 3 {
		t.Errorf("gained %d, want 3 from the crewer's trigger only", got)
	}
}

// Card.subtractCounter (Card.java:1935-1945): CounterRemoved fires once per
// counter taken off, NewCounterAmount counting down (0 on the last), and
// CounterRemovedOnce once for the whole removal.
func TestCounterRemovedTriggersFollowRemovedCounters(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGame(t)
	host := g.NewCard(scriptDef(t, "Test Hydra", "Creature Hydra",
		"T:Mode$ CounterRemoved | ValidCard$ Card.Self | CounterType$ P1P1 | TriggerZones$ Battlefield | Execute$ TrigEach",
		"T:Mode$ CounterRemoved | ValidCard$ Card.Self | CounterType$ P1P1 | NewCounterAmount$ 0 | TriggerZones$ Battlefield | Execute$ TrigLast",
		"T:Mode$ CounterRemovedOnce | ValidCard$ Card.Self | CounterType$ P1P1 | TriggerZones$ Battlefield | Execute$ TrigOnce",
		"SVar:TrigEach:DB$ GainLife | Defined$ You | LifeAmount$ 1",
		"SVar:TrigLast:DB$ GainLife | Defined$ You | LifeAmount$ 10",
		"SVar:TrigOnce:DB$ GainLife | Defined$ You | LifeAmount$ 100"), p, engine.Battlefield)
	g.Card(host).Counters.Add(engine.CounterType("P1P1"), 3)
	engine.CheckStateBasedActions(g, engine.NewScriptedController())
	before := g.Player(p).Life
	c := engine.NewScriptedController()
	c.QueueTargets([]engine.EntityID{engine.CardEntity(host)})

	if _, err := castETBChain(t, g, p, etbChainDef(t, "Test Remover",
		"DB$ RemoveCounter | ValidTgts$ Creature | CounterType$ P1P1 | CounterNum$ 3"), c); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}

	// 3 x 1 (each counter) + 10 (the last) + 100 (the removal).
	if got := g.Player(p).Life - before; got != 113 {
		t.Errorf("gained %d, want 113", got)
	}
}

// TestManaExpendFiresAsSpentManaCrossesAmount: a four-mana spell crosses the
// Amount$ 4 threshold of a "spend your fourth mana" permanent once; a
// following spell spending one more mana does not fire it again.
func TestManaExpendFiresAsSpentManaCrossesAmount(t *testing.T) {
	t.Parallel()

	g, p, other := newTwoPlayerGame(t)
	watcher := scriptDef(t, "Expend Watcher", "Creature Elf",
		"T:Mode$ ManaExpend | Player$ You | Amount$ 4 | TriggerZones$ Battlefield | Execute$ TrigGain | TriggerDescription$ x",
		"SVar:TrigGain:DB$ GainLife | Defined$ You | LifeAmount$ 3")
	g.NewCard(watcher, p, engine.Battlefield)
	spell := scriptDef(t, "Four Drop", "Creature Elf")
	spell.Faces[0].ManaCost = mana.MustParse("3 G")
	c := engine.NewScriptedController()
	g.Player(p).ManaPool.Add(mana.Green, 8)
	for i := 0; i < 2; i++ {
		c.QueuePayGeneric(mana.ShardG)
		c.QueuePayGeneric(mana.ShardG)
		c.QueuePayGeneric(mana.ShardG)
		if !g.CastSpell(p, g.NewCard(spell, p, engine.Hand), c) {
			t.Fatal("CastSpell failed")
		}
		if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
			t.Fatalf("ResolveStack: %v", err)
		}
	}
	if got := g.Player(p).Life; got != 23 {
		t.Errorf("life = %d, want 23 (one trigger: 4 mana spent, then 4 more crosses 8 not 4)", got)
	}
	_ = other
}
