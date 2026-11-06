package engine_test

import (
	"strconv"
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
	"github.com/jczastkiewicz/crucible/internal/mana"
)

// CR 613.3: within a layer, characteristic-defining abilities apply first,
// then the rest in timestamp order. Transguild Courier ("is all colors", a CDA)
// entering after Darkest Hour ("all creatures are black") is still black: the
// older non-CDA effect applies after the newer CDA.
func TestCDAColorAppliesBeforeAnOlderColorChange(t *testing.T) {
	t.Parallel()
	g, p, _ := newTwoPlayerGameOn(t, scenarioDB(t))
	g.NewCard(corpusCard(t, "Darkest Hour"), p, engine.Battlefield)
	courier := g.NewCard(corpusCard(t, "Transguild Courier"), p, engine.Battlefield)
	engine.CheckStateBasedActions(g, engine.NewScriptedController())
	if got := g.Card(courier).Colors(); got != mana.Black {
		t.Errorf("Transguild Courier colors %v, want black (CDA first, then Darkest Hour)", got)
	}
}

// CR 613.3 on the type line: Mistform Ultimus's "is every creature type" CDA
// applies on the battlefield and off it (CR 604.3).
func TestCDATypeAppliesInEveryZone(t *testing.T) {
	t.Parallel()
	g, p, _ := newTwoPlayerGameOn(t, scenarioDB(t))
	for _, zone := range []engine.ZoneType{engine.Battlefield, engine.Hand} {
		ultimus := g.NewCard(corpusCard(t, "Mistform Ultimus"), p, zone)
		engine.CheckStateBasedActions(g, engine.NewScriptedController())
		if !g.Card(ultimus).Type().HasSubtype("Elf") {
			t.Errorf("Mistform Ultimus in %v is not an Elf, want every creature type", zone)
		}
	}
}

// CR 613.7b: an effect from a resolved ability is timestamped when it is
// created, so of two base power/toughness settings the later wins, whichever
// value it sets.
func TestResolvedEffectsApplyInResolutionOrder(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		first, second, want string
	}{
		{"1", "4", "4"},
		{"4", "1", "1"},
	} {
		g, p, _ := newTwoPlayerGame(t)
		bear := g.NewCard(creatureDefPT(t, "2", "2"), p, engine.Battlefield)
		for _, n := range []string{tc.first, tc.second} {
			c := engine.NewScriptedController()
			c.QueueTargets([]engine.EntityID{engine.CardEntity(bear)})
			def := etbChainDef(t, "Test Setter "+n, "DB$ Animate | ValidTgts$ Creature | Power$ "+n+" | Toughness$ "+n)
			if _, err := castETBChain(t, g, p, def, c); err != nil {
				t.Fatalf("ResolveStack: %v", err)
			}
		}
		pw, _ := g.Card(bear).Power()
		if got := strconv.Itoa(pw); got != tc.want {
			t.Errorf("set %s then %s: power %s, want the later %s", tc.first, tc.second, got, tc.want)
		}
	}
}

// CR 613.7e: a permanent turned face up gets a new timestamp, later than a
// card that entered after it was cast face down.
func TestTurningFaceUpGivesANewTimestamp(t *testing.T) {
	t.Parallel()
	g, p, _ := newTwoPlayerGameOn(t, scenarioDB(t))
	g.SetTurnState(1, p, engine.Main1)
	bane := g.NewCard(corpusCard(t, "Bane of the Living"), p, engine.Hand)
	castFaceDown(t, g, p, bane)
	later := g.NewCard(creatureDefPT(t, "1", "1"), p, engine.Battlefield)
	g.Player(p).ManaPool.Add(mana.Black, 2)
	c := engine.NewScriptedController()
	c.QueuePayX(0)
	if !g.TurnFaceUp(p, bane, 0, c) {
		t.Fatalf("turn face up failed: %v", g.TakePendingError())
	}
	if g.Card(bane).Timestamp <= g.Card(later).Timestamp {
		t.Errorf("timestamp %d after turning face up, want past the later card's %d", g.Card(bane).Timestamp, g.Card(later).Timestamp)
	}
}

// CR 613.7f: a double-faced permanent that transforms gets a new timestamp.
func TestTransformingGivesANewTimestamp(t *testing.T) {
	t.Parallel()
	g, p, _ := newTwoPlayerGameOn(t, scenarioDB(t))
	delver := g.NewCard(corpusCard(t, "Delver of Secrets"), p, engine.Battlefield)
	later := g.NewCard(creatureDefPT(t, "1", "1"), p, engine.Battlefield)
	if !g.Transform(delver) {
		t.Fatal("transform failed")
	}
	if g.Card(delver).Timestamp <= g.Card(later).Timestamp {
		t.Errorf("timestamp %d after transforming, want past the later card's %d", g.Card(delver).Timestamp, g.Card(later).Timestamp)
	}
}

// CR 505.1 and 507.1 (PhaseHandler.java:436-441): "at the beginning of your
// precombat main phase" and "at the beginning of combat on your turn"
// triggers fire as their step begins, once.
func TestBeginningOfStepTriggersFireOnTheirStep(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		phase string
		step  engine.PhaseType
	}{
		{"Main1", engine.Main1},
		{"BeginCombat", engine.CombatBegin},
	} {
		g, p, other := newTwoPlayerGame(t)
		g.Player(p).Life = 20
		watcher := planarDef(t, "Test "+tc.phase+" Watcher", "Enchantment",
			[]string{"Mode$ Phase | Phase$ " + tc.phase + " | ValidPlayer$ You | TriggerZones$ Battlefield | Execute$ TrigGain"}, nil,
			"TrigGain", "DB$ GainLife | Defined$ You | LifeAmount$ 5")
		g.NewCard(watcher, p, engine.Battlefield)
		libraryCards(t, g, p, 3)
		g.SetTurnState(2, other, engine.Cleanup)
		c := engine.NewScriptedController()
		toStep(t, g, c, p, tc.step, nil)
		if got := g.Player(p).Life; got != 25 {
			t.Errorf("%s: life %d on reaching the step, want 25", tc.phase, got)
		}
		toStep(t, g, c, p, engine.CombatEnd, nil)
		if got := g.Player(p).Life; got != 25 {
			t.Errorf("%s: life %d at end of combat, want the trigger once", tc.phase, got)
		}
	}
}
