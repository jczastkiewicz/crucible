package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
	"github.com/jczastkiewicz/crucible/internal/mana"
)

func planGame(t *testing.T, names ...string) (*engine.Game, engine.PlayerID, []engine.CardID) {
	t.Helper()
	g, p, _ := newTwoPlayerGameOn(t, scenarioDB(t))
	ids := make([]engine.CardID, len(names))
	for i, n := range names {
		ids[i] = g.NewCard(corpusCard(t, n), p, engine.Battlefield)
		g.Card(ids[i]).SummonSick = false
	}
	return g, p, ids
}

func planCost(t *testing.T, s string) mana.Cost {
	t.Helper()
	c, err := mana.Parse(s)
	if err != nil {
		t.Fatalf("mana.Parse(%q): %v", s, err)
	}
	return c
}

func TestPlanManaPaysFromLandsByColour(t *testing.T) {
	t.Parallel()
	g, p, _ := planGame(t, "Mountain", "Mountain", "Island")
	for _, tc := range []struct {
		cost string
		want bool
	}{
		{"1 R", true}, {"U R", true}, {"R R", true}, {"R R U", true},
		{"R R R", false}, {"B", false}, {"2 2", false}, {"R R U 1", false},
		{"R/U", true}, {"B/G", false}, {"B/U", true},
	} {
		if _, got := g.PlanMana(p, planCost(t, tc.cost)); got != tc.want {
			t.Errorf("PlanMana(%q) = %v, want %v", tc.cost, got, tc.want)
		}
	}
	if _, ok := g.PlanMana(p, mana.NoCost()); ok {
		t.Error("a card with no mana cost was payable")
	}
}

func TestPlanManaKeepsFlexibleSourcesForColouredSymbols(t *testing.T) {
	t.Parallel()
	// Birds of Paradise makes any colour; Mountain only red. {1}{R}{G}... the
	// plan must give Birds the green and the Mountain the red.
	g, p, _ := planGame(t, "Mountain", "Birds of Paradise", "Island")
	plan, ok := g.PlanMana(p, planCost(t, "1 R G"))
	if !ok {
		t.Fatal("{1}{R}{G} not payable with Mountain, Birds, Island")
	}
	if len(plan.Taps) != 3 {
		t.Fatalf("%d taps, want 3", len(plan.Taps))
	}
	if _, ok := g.PlanMana(p, planCost(t, "R R G")); ok {
		t.Error("{R}{R}{G} paid without two red sources")
	}
}

func TestPlanManaSkipsSickCreaturesAndTappedLands(t *testing.T) {
	t.Parallel()
	g, p, ids := planGame(t, "Llanowar Elves", "Forest")
	g.Card(ids[0]).SummonSick = true
	if _, ok := g.PlanMana(p, planCost(t, "G G")); ok {
		t.Error("a summoning-sick Elves made mana")
	}
	g.Card(ids[0]).SummonSick = false
	if _, ok := g.PlanMana(p, planCost(t, "G G")); !ok {
		t.Error("Elves and Forest did not make {G}{G}")
	}
	g.Card(ids[1]).Tapped = true
	if _, ok := g.PlanMana(p, planCost(t, "G G")); ok {
		t.Error("a tapped Forest made mana")
	}
}

func TestPlanManaFallbacksForPhyrexianAndTwoGenericHybrid(t *testing.T) {
	t.Parallel()
	g, p, _ := planGame(t, "Mountain", "Mountain")
	plan, ok := g.PlanMana(p, planCost(t, "W/P"))
	if !ok || plan.Life != 2 || len(plan.Taps) != 0 {
		t.Errorf("{W/P} with no white: plan %+v ok %v, want 2 life and no taps", plan, ok)
	}
	plan, ok = g.PlanMana(p, planCost(t, "2/W"))
	if !ok || len(plan.Taps) != 2 {
		t.Errorf("{2/W} with two Mountains: plan %+v ok %v, want two taps", plan, ok)
	}
	if _, ok := g.PlanMana(p, planCost(t, "2/W 1")); ok {
		t.Error("{2/W}{1} paid with two sources")
	}
}

func TestPlanManaAnyColourTellsColourFromQuantity(t *testing.T) {
	t.Parallel()
	g, p, _ := planGame(t, "Mountain", "Mountain")
	if _, ok := g.PlanMana(p, planCost(t, "W W")); ok {
		t.Error("{W}{W} paid from Mountains")
	}
	if _, ok := g.PlanManaAnyColor(p, planCost(t, "W W")); !ok {
		t.Error("two sources did not pay {W}{W} as any colour")
	}
	if _, ok := g.PlanManaAnyColor(p, planCost(t, "W W W")); ok {
		t.Error("three mana from two sources")
	}
}

func TestTapForPlanPutsTheManaInThePool(t *testing.T) {
	t.Parallel()
	g, p, ids := planGame(t, "Mountain", "Birds of Paradise", "Island")
	plan, ok := g.PlanMana(p, planCost(t, "1 R G"))
	if !ok {
		t.Fatal("no plan")
	}
	if !g.TapForPlan(p, plan, engine.NewScriptedController()) {
		t.Fatal("TapForPlan failed")
	}
	for _, id := range ids {
		if !g.Card(id).Tapped {
			t.Errorf("%v was not tapped", g.Card(id).Name())
		}
	}
	if got := g.Player(p).ManaPool.Total(); got != 3 {
		t.Errorf("pool holds %d mana, want 3", got)
	}
	if g.TapForPlan(p, plan, engine.NewScriptedController()) {
		t.Error("tapping the same sources twice succeeded")
	}
}
