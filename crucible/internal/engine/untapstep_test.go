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

// Mode$ BecomesCrewed fires once as the Crew ability resolves (not per
// crewer); FirstTimeCrewed$ holds only for the first crew of the turn.
func TestBecomesCrewedFiresOncePerCrewAndFirstTimeOnlyFirst(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGame(t)
	vehicle := g.NewCard(scriptDef(t, "Test Vehicle", "Artifact Vehicle", "K:Crew:1",
		"T:Mode$ BecomesCrewed | ValidCard$ Card.Self | Execute$ TrigEach | TriggerZones$ Battlefield",
		"T:Mode$ BecomesCrewed | ValidCard$ Card.Self | FirstTimeCrewed$ True | Execute$ TrigFirst | TriggerZones$ Battlefield",
		"SVar:TrigEach:DB$ GainLife | Defined$ You | LifeAmount$ 1",
		"SVar:TrigFirst:DB$ GainLife | Defined$ You | LifeAmount$ 10"), p, engine.Battlefield)
	crewers := tappedCreatures(t, g, p, 2)
	for _, id := range crewers {
		g.Card(id).Tapped = false
	}
	engine.CheckStateBasedActions(g, engine.NewScriptedController())
	before := g.Player(p).Life
	c := engine.NewScriptedController()
	for i, id := range crewers {
		c.QueueCardChoice([]engine.CardID{id})
		if !g.ActivateAbility(p, vehicle, 0, c) {
			t.Fatalf("Crew %d could not be activated", i)
		}
		if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
			t.Fatalf("ResolveStack: %v", err)
		}
	}

	// Crew 1: 1 + 10; crew 2: 1.
	if got := g.Player(p).Life - before; got != 12 {
		t.Errorf("gained %d, want 12", got)
	}
}

// Mode$ UntapAll fires once for the permanents the untap step untapped.
func TestUntapAllFiresOnceForTheUntapStep(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGame(t)
	g.NewCard(scriptDef(t, "Test Calendar", "Artifact",
		"T:Mode$ UntapAll | ValidCards$ Permanent | ValidPlayer$ You | Phase$ Untap | PlayerTurn$ True | TriggerZones$ Battlefield | Execute$ TrigGain",
		"SVar:TrigGain:DB$ GainLife | Defined$ You | LifeAmount$ 7"), p, engine.Battlefield)
	tappedCreatures(t, g, p, 3)
	c := engine.NewScriptedController()
	before := g.Player(p).Life

	g.StartTurn(p, c)
	if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}

	if got := g.Player(p).Life - before; got != 7 {
		t.Errorf("gained %d, want 7 (one run for three untapped creatures)", got)
	}
}

// K:Saddle:N taps creatures of total power N to saddle a Mount: it becomes
// saddled (IsSaddled), the Saddled trigger fires for the creature tapped and
// BecomesSaddled once, FirstTimeSaddled$ only for the first saddle.
func TestSaddleSaddlesTheMountAndFiresTriggers(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGame(t)
	mount := g.NewCard(scriptDef(t, "Test Mount", "Creature Mount", "K:Saddle:1",
		"T:Mode$ BecomesSaddled | ValidSaddled$ Card.Self | Execute$ TrigEach | TriggerZones$ Battlefield",
		"T:Mode$ BecomesSaddled | ValidSaddled$ Card.Self | FirstTimeSaddled$ True | Execute$ TrigFirst | TriggerZones$ Battlefield",
		"T:Mode$ Saddled | ValidCard$ Card.Self | ValidCrew$ Creature | Execute$ TrigPer | TriggerZones$ Battlefield",
		"SVar:TrigEach:DB$ GainLife | Defined$ You | LifeAmount$ 1",
		"SVar:TrigFirst:DB$ GainLife | Defined$ You | LifeAmount$ 10",
		"SVar:TrigPer:DB$ GainLife | Defined$ You | LifeAmount$ 100"), p, engine.Battlefield)
	riders := tappedCreatures(t, g, p, 1)
	g.Card(riders[0]).Tapped = false
	engine.CheckStateBasedActions(g, engine.NewScriptedController())
	before := g.Player(p).Life
	c := engine.NewScriptedController()
	c.QueueCardChoice([]engine.CardID{riders[0]})

	if !g.ActivateAbility(p, mount, 0, c) {
		t.Fatal("Saddle could not be activated")
	}
	if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}

	if !g.Card(mount).Saddled {
		t.Error("the Mount is not saddled")
	}
	if got := g.Player(p).Life - before; got != 111 {
		t.Errorf("gained %d, want 111 (Saddled 100 + BecomesSaddled 1 + first time 10)", got)
	}
}

// CombatMana$ mana survives the end of each step until the combat phase
// ends, and is gone once Main2 begins.
func TestCombatManaSurvivesUntilCombatEnds(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGame(t)
	c := engine.NewScriptedController()
	g.Player(p).ManaPool.Empty()
	resolveLine(t, g, p, c, "DB$ Mana | Produced$ R | Amount$ 2 | CombatMana$ True")
	g.Player(p).ManaPool.Add(mana.Green, 1)

	sawCombat := false
	for g.ActivePhase() != engine.Main2 {
		g.AdvancePhase(c)
		if g.ActivePhase() == engine.CombatEnd {
			sawCombat = true
			if got := g.Player(p).ManaPool.Total(); got != 2 {
				t.Errorf("pool at EndCombat = %d, want the 2 combat mana", got)
			}
		}
	}
	if !sawCombat {
		t.Fatal("never reached EndCombat")
	}
	if got := g.Player(p).ManaPool.Total(); got != 0 {
		t.Errorf("pool at Main2 = %d, want 0", got)
	}
}

// Sacrificed$ChromaSource.Red: the red mana symbols in each sacrificed card's
// mana cost.
func TestChromaSourceWithColorCountsSacrificedSymbols(t *testing.T) {
	t.Parallel()

	g, p, other := newTwoPlayerGame(t)
	fodder := scriptDef(t, "Test Fodder", "Creature Goblin")
	fodder.Faces[0].ManaCost = mana.MustParse("1 R R")
	victim := g.NewCard(fodder, p, engine.Battlefield)
	c := engine.NewScriptedController()
	c.QueueSacrificeChoice([]engine.CardID{victim})

	resolveLine(t, g, p, c,
		"DB$ Sacrifice | SacValid$ Creature.Goblin | RememberSacrificed$ True | SubAbility$ DBHit",
		"DBHit", "DB$ LoseLife | Defined$ Player.Opponent | LifeAmount$ X",
		"X", "Remembered$ChromaSource.Red")

	if got := g.Player(other).Life; got != 18 {
		t.Errorf("opponent life = %d, want 18 (two red symbols)", got)
	}
}

// ChooseCard WithDifferentPowers$ (Sigarda's Vanguard): each pick drops every
// creature of that power from the next offer, so the chosen creatures all have
// different powers.
func TestChooseCardWithDifferentPowersExcludesChosenPowers(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGame(t)
	a := g.NewCard(creatureDefPT(t, "2", "2"), p, engine.Battlefield)
	b := g.NewCard(creatureDefPT(t, "2", "2"), p, engine.Battlefield)
	d := g.NewCard(creatureDefPT(t, "3", "3"), p, engine.Battlefield)
	c := engine.NewScriptedController()
	c.QueueCardChoice([]engine.CardID{a})
	c.QueueCardChoice([]engine.CardID{d})

	host := resolveLine(t, g, p, c, "DB$ ChooseCard | MinAmount$ 0 | Amount$ 5 | Choices$ Creature.YouCtrl+Other | WithDifferentPowers$ True")

	chosen := g.Card(host).Memory.Chosen()
	if len(chosen) != 2 || chosen[0] != a || chosen[1] != d {
		t.Errorf("chosen = %v, want [%v %v] (b shares a's power)", chosen, a, d)
	}
	_ = b
}
