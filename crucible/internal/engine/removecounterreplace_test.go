package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
)

// Event$ RemoveCounter with Layer$ CantHappen (Card.canRemoveCounters,
// Card.java:1737-1749): Fear of Sleep Paralysis keeps stun counters on the
// permanents of its controller's opponents, and only those.
func TestRemoveCounterCantHappenKeepsMatchingCounters(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name    string
		counter string
		mine    bool
		want    int
	}{
		{"opponent's stun counter stays", "Stun", false, 2},
		{"another kind comes off", "P1P1", false, 1},
		{"own permanent's stun counter comes off", "Stun", true, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			g, p, other := newTwoPlayerGame(t)
			g.NewCard(scriptDef(t, "Test Paralysis", "Enchantment",
				"R:Event$ RemoveCounter | ValidCard$ Permanent.OppCtrl | ValidCounterType$ STUN | Layer$ CantHappen"), p, engine.Battlefield)
			owner := other
			if tc.mine {
				owner = p
			}
			target := g.NewCard(creatureDefPT(t, "2", "2"), owner, engine.Battlefield)
			g.Card(target).Counters.Add(engine.CounterType(map[string]string{"Stun": "STUN", "P1P1": "P1P1"}[tc.counter]), 2)
			line := "DB$ RemoveCounter | ValidTgts$ Creature | CounterType$ " + tc.counter + " | CounterNum$ 1"
			if _, err := resolveNow(t, g, p, engine.NewScriptedController(), []engine.EntityID{engine.CardEntity(target)}, line); err != nil {
				t.Fatal(err)
			}
			kind := engine.CounterType(map[string]string{"Stun": "STUN", "P1P1": "P1P1"}[tc.counter])
			if got := g.Card(target).Counters.Count(kind); got != tc.want {
				t.Errorf("%s counters = %d, want %d", tc.counter, got, tc.want)
			}
		})
	}
}

// Deification (Card.java:1894): loyalty lost to damage is offered to the
// RemoveCounter replacement as Result, and ReplaceEffect sets what stays.
func TestRemoveCounterReplacementReducesLoyaltyLoss(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name       string
		withHelper bool
		damage     int
		want       int
	}{
		{"keeps one loyalty", true, 5, 1},
		{"damage short of lethal is untouched", true, 2, 1},
		{"condition unmet", false, 5, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			g, p, _ := newTwoPlayerGame(t)
			g.NewCard(scriptDef(t, "Test Deification", "Enchantment",
				"R:Event$ RemoveCounter | ValidCard$ Planeswalker.YouCtrl | Result$ LT1 | ValidCounterType$ LOYALTY | IsDamage$ True | ReplaceWith$ ReduceLoss | IsPresent$ Artifact.YouCtrl",
				"SVar:ReduceLoss:DB$ ReplaceEffect | VarName$ Result | VarValue$ 1"), p, engine.Battlefield)
			if tc.withHelper {
				g.NewCard(scriptDef(t, "Test Helper", "Artifact"), p, engine.Battlefield)
			}
			walker := g.NewCard(scriptDef(t, "Test Walker", "Planeswalker"), p, engine.Battlefield)
			g.Card(walker).Counters.Add(engine.Loyalty, 3)
			line := "DB$ DealDamage | NumDmg$ " + string(rune('0'+tc.damage)) + " | ValidTgts$ Any"
			if _, err := resolveNow(t, g, p, engine.NewScriptedController(), []engine.EntityID{engine.CardEntity(walker)}, line); err != nil {
				t.Fatal(err)
			}
			if c := g.Card(walker); c.Zone == engine.Battlefield {
				if got := c.Counters.Count(engine.Loyalty); got != tc.want {
					t.Errorf("loyalty = %d, want %d", got, tc.want)
				}
			} else if tc.want != 0 {
				t.Errorf("walker left the battlefield, want loyalty %d", tc.want)
			}
		})
	}
}
