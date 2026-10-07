package engine_test

import (
	"strings"
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
	"github.com/jczastkiewicz/crucible/internal/mana"
)

// DefinedCost_<Defined>[_Minus<N>|_Plus<N>] (AbilityUtils.java:1450-1471): the
// payer is charged the named card's mana cost, its generic part changed.
func TestUnlessCostDefinedCostIsTheNamedCardsManaCost(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name    string
		cost    string
		pool    int // green mana on top of the {G} the cast spends
		generic int // queued generic payments
		pay     bool
		want    engine.ZoneType
	}{
		{"self cost paid", "DefinedCost_Self", 1, 0, true, engine.Battlefield},
		{"self cost declined", "DefinedCost_Self", 1, 0, false, engine.Graveyard},
		{"plus raises the generic part", "DefinedCost_Self_Plus1", 2, 1, true, engine.Battlefield},
		{"plus unaffordable", "DefinedCost_Self_Plus1", 1, 1, true, engine.Graveyard},
		{"minus never goes below zero", "DefinedCost_Self_Minus2", 1, 0, true, engine.Battlefield},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			g := newGame(t, "a", "b")
			p := g.Players()[0]
			g.SetTurnState(1, p, engine.Main1)
			g.Player(p).Life, g.Player(g.Players()[1]).Life = 20, 20
			g.Player(p).ManaPool.Add(mana.Green, tc.pool)
			c := engine.NewScriptedController()
			c.QueueConfirmPayCost(tc.pay)
			queueXPayGeneric(c, mana.ShardG, tc.generic)
			def := etbSacrificeTriggerDefParams(t, "Test Defined Cost", "UnlessCost$ "+tc.cost+" | UnlessPayer$ You", nil)

			creature, err := castETBSacrifice(t, g, p, def, c)
			if err != nil {
				t.Fatalf("ResolveStack: %v", err)
			}
			if z := g.Card(creature).Zone; z != tc.want {
				t.Errorf("creature zone = %v, want %v", z, tc.want)
			}
		})
	}
}

// A DefinedCost_ naming no card is calculateUnlessCost returning null: nobody is
// asked and the ability resolves.
func TestUnlessCostDefinedCostNamingNothingResolvesWithoutAsking(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p := g.Players()[0]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life, g.Player(g.Players()[1]).Life = 20, 20
	c := engine.NewScriptedController()
	c.QueueConfirmPayCost(true)
	def := etbSacrificeTriggerDefParams(t, "Test Defined Nothing", "UnlessCost$ DefinedCost_Remembered_Minus2 | UnlessPayer$ You", nil)
	creature, err := castETBSacrifice(t, g, p, def, c)
	if err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if z := g.Card(creature).Zone; z != engine.Graveyard {
		t.Errorf("creature zone = %v, want Graveyard: no card, no cost, the sacrifice runs", z)
	}
}

// UnlessUpTo$ asks the payer how much of the reduction to take, which this port
// does not model: the line fails loudly rather than guess.
func TestUnlessCostDefinedCostWithUnlessUpToFailsClosed(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p := g.Players()[0]
	g.SetTurnState(1, p, engine.Main1)
	def := etbSacrificeTriggerDefParams(t, "Test Up To", "UnlessCost$ DefinedCost_Self_Minus1 | UnlessUpTo$ True | UnlessPayer$ You", nil)
	_, err := castETBSacrifice(t, g, p, def, engine.NewScriptedController())
	if err == nil || !strings.Contains(err.Error(), "UnlessUpTo") {
		t.Fatalf("ResolveStack error = %v, want one naming UnlessUpTo$", err)
	}
}

// A DefinedCost_ that is not a recognised modifier or whose card has no plain
// cost is an error, never a free payment.
func TestUnlessCostDefinedCostBadModifierFails(t *testing.T) {
	t.Parallel()

	for _, cost := range []string{"DefinedCost_Self_Times2", "DefinedCost_Self_MinusX", "DefinedCost_Self_PlusX", "DefinedCost_Bogus"} {
		t.Run(cost, func(t *testing.T) {
			t.Parallel()

			g := newGame(t, "a", "b")
			p := g.Players()[0]
			g.SetTurnState(1, p, engine.Main1)
			def := etbSacrificeTriggerDefParams(t, "Test Bad Modifier", "UnlessCost$ "+cost+" | UnlessPayer$ You", nil)
			if _, err := castETBSacrifice(t, g, p, def, engine.NewScriptedController()); err == nil {
				t.Fatal("ResolveStack error = nil, want an unresolvable cost")
			}
		})
	}
}

// An absent UnlessPayer$ is TargetedController: the controller of the targeted
// card is the one asked (AbilityUtils.java:1407, Perplex).
func TestUnlessCostDefaultPayerIsTheTargetedCardsController(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name string
		pay  bool
		want engine.ZoneType
	}{
		{"targeted player pays", true, engine.Battlefield},
		{"targeted player declines", false, engine.Graveyard},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			g, p, other := newTwoPlayerGame(t)
			victim := g.NewCard(creatureDefPT(t, "2", "2"), other, engine.Battlefield)
			g.Player(other).ManaPool.Add(mana.Green, 2)
			c := engine.NewScriptedController()
			c.QueueTargets([]engine.EntityID{engine.CardEntity(victim)})
			c.QueueConfirmPayCost(tc.pay)
			queueXPayGeneric(c, mana.ShardG, 2)
			resolveLine(t, g, p, c, "DB$ Destroy | ValidTgts$ Creature.OppCtrl | UnlessCost$ 2")
			if z := g.Card(victim).Zone; z != tc.want {
				t.Errorf("victim zone = %v, want %v", z, tc.want)
			}
			if tc.pay && g.Player(other).ManaPool.Total() != 0 {
				t.Errorf("the targeted card's controller kept %d mana after paying", g.Player(other).ManaPool.Total())
			}
		})
	}
}

// PutCardToLibFromGrave<N/-1/Card>: the payer puts N cards from their graveyard
// on the bottom of their library (CostPutCardToLib).
func TestUnlessCostPutCardToLibFromGrave(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name       string
		graveyard  int
		pay        bool
		wantSaved  bool
		wantLibLen int
	}{
		{"paid", 2, true, true, 3},
		{"declined", 2, false, false, 1},
		{"too few cards", 1, true, false, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			g := newGame(t, "a", "b")
			p := g.Players()[0]
			g.SetTurnState(1, p, engine.Main1)
			g.Player(p).Life, g.Player(g.Players()[1]).Life = 20, 20
			var grave []engine.CardID
			for range tc.graveyard {
				grave = append(grave, g.NewCard(creatureDefPT(t, "1", "1"), p, engine.Graveyard))
			}
			bottom := g.NewCard(creatureDefPT(t, "1", "1"), p, engine.Library)
			c := engine.NewScriptedController()
			c.QueueConfirmPayCost(tc.pay)
			c.QueueCardChoice(grave)
			def := etbSacrificeTriggerDefParams(t, "Test Put To Lib", "UnlessCost$ PutCardToLibFromGrave<2/-1/Card> | UnlessPayer$ You", nil)
			creature, err := castETBSacrifice(t, g, p, def, c)
			if err != nil {
				t.Fatalf("ResolveStack: %v", err)
			}
			if saved := g.Card(creature).Zone == engine.Battlefield; saved != tc.wantSaved {
				t.Errorf("creature survived = %v, want %v", saved, tc.wantSaved)
			}
			lib := g.Zone(engine.Library, p).Cards()
			if len(lib) != tc.wantLibLen {
				t.Fatalf("library = %d cards, want %d", len(lib), tc.wantLibLen)
			}
			if tc.wantSaved && (lib[0] != bottom || lib[1] != grave[0] || lib[2] != grave[1]) {
				t.Errorf("library order = %v, want the chosen cards under %v", lib, bottom)
			}
		})
	}
}

// RemoveAnyCounter<N/Any/Permanent> (CostRemoveAnyCounter): the payer removes
// counters from among their permanents, choosing the kind when there are several.
func TestUnlessCostRemoveAnyCounter(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name      string
		pay       bool
		counters  int
		wantSaved bool
		wantLeft  int
	}{
		{"paid", true, 2, true, 1},
		{"declined", false, 2, false, 2},
		{"no counter anywhere", true, 0, false, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			g := newGame(t, "a", "b")
			p := g.Players()[0]
			g.SetTurnState(1, p, engine.Main1)
			g.Player(p).Life, g.Player(g.Players()[1]).Life = 20, 20
			holder := g.NewCard(creatureDefPT(t, "1", "1"), p, engine.Battlefield)
			g.Card(holder).Counters.Add(engine.Charge, tc.counters)
			c := engine.NewScriptedController()
			c.QueueConfirmPayCost(tc.pay)
			c.QueueCardChoice([]engine.CardID{holder})
			def := etbSacrificeTriggerDefParams(t, "Test Remove Any", "UnlessCost$ RemoveAnyCounter<1/Any/Permanent> | UnlessPayer$ You", nil)
			creature, err := castETBSacrifice(t, g, p, def, c)
			if err != nil {
				t.Fatalf("ResolveStack: %v", err)
			}
			if saved := g.Card(creature).Zone == engine.Battlefield; saved != tc.wantSaved {
				t.Errorf("creature survived = %v, want %v", saved, tc.wantSaved)
			}
			if n := g.Card(holder).Counters.Count(engine.Charge); n != tc.wantLeft {
				t.Errorf("counters left = %d, want %d", n, tc.wantLeft)
			}
		})
	}
}

// With several kinds on the chosen permanent the payer picks which one to
// remove, and a named kind removes only that kind.
func TestUnlessCostRemoveAnyCounterKinds(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name      string
		spec      string
		option    int
		wantP1P1  int
		wantCharg int
	}{
		{"payer picks the second kind", "Any", 1, 1, 0},
		{"payer picks the first kind", "Any", 0, 0, 1},
		{"a named kind", "P1P1", 0, 0, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			g := newGame(t, "a", "b")
			p := g.Players()[0]
			g.SetTurnState(1, p, engine.Main1)
			g.Player(p).Life, g.Player(g.Players()[1]).Life = 20, 20
			holder := g.NewCard(creatureDefPT(t, "1", "1"), p, engine.Battlefield)
			g.Card(holder).Counters.Add(engine.P1P1, 1)
			g.Card(holder).Counters.Add(engine.Charge, 1)
			c := engine.NewScriptedController()
			c.QueueConfirmPayCost(true)
			c.QueueCardChoice([]engine.CardID{holder})
			c.QueueOption(tc.option)
			def := etbSacrificeTriggerDefParams(t, "Test Remove Kind", "UnlessCost$ RemoveAnyCounter<1/"+tc.spec+"/Permanent> | UnlessPayer$ You", nil)
			if _, err := castETBSacrifice(t, g, p, def, c); err != nil {
				t.Fatalf("ResolveStack: %v", err)
			}
			if n := g.Card(holder).Counters.Count(engine.P1P1); n != tc.wantP1P1 {
				t.Errorf("P1P1 left = %d, want %d", n, tc.wantP1P1)
			}
			if n := g.Card(holder).Counters.Count(engine.Charge); n != tc.wantCharg {
				t.Errorf("CHARGE left = %d, want %d", n, tc.wantCharg)
			}
		})
	}
}

// Mode$ CollectEvidence (TriggerCollectEvidence, CostCollectEvidence.java:78)
// fires for the player who paid the cost; ValidPlayer$ names who.
func TestCollectEvidenceTriggerFiresForThePayer(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name     string
		line     string
		wantHand int
	}{
		{"watcher for you", "Mode$ CollectEvidence | ValidPlayer$ You | TriggerZones$ Battlefield", 1},
		{"watcher for opponents", "Mode$ CollectEvidence | ValidPlayer$ Opponent | TriggerZones$ Battlefield", 0},
		{"a param it does not read", "Mode$ CollectEvidence | ValidPlayer$ You | Bogus$ True | TriggerZones$ Battlefield", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			g, p, _ := watcherGame(t, tc.line)
			big := g.NewCard(creatureDefManaCost(t, "3"), p, engine.Graveyard)
			c := engine.NewScriptedController()
			c.QueueConfirmPayCost(true)
			c.QueueCardChoice([]engine.CardID{big})
			def := etbSacrificeTriggerDefParams(t, "Test Evidence", "UnlessCost$ CollectEvidence<3> | UnlessPayer$ You", nil)
			if _, err := castETBSacrifice(t, g, p, def, c); err != nil {
				t.Fatalf("ResolveStack: %v", err)
			}
			if got := g.Zone(engine.Hand, p).Len(); got != tc.wantHand {
				t.Errorf("hand = %d, want %d", got, tc.wantHand)
			}
		})
	}
}
