package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
)

// CR 603.4: a triggered ability with an "intervening if" checks the
// condition again as it resolves (WrappedAbility.java:402-412). "At the
// beginning of your upkeep, if you control no creatures, you may gain 3 life"
// triggers on an empty board; a creature arriving before it resolves makes it
// do nothing, and its "you may" is never asked (no answer is queued, so
// asking would fail the test). NoResolvingCheck$ True skips the recheck.
func TestInterveningIfIsRecheckedOnResolution(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name         string
		extra        string
		creatureCame bool
		want         int
	}{
		{"condition still holds", "", false, 23},
		{"condition broken before resolution", "", true, 20},
		{"NoResolvingCheck", " | NoResolvingCheck$ True", true, 23},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			g, p, _ := newTwoPlayerGame(t)
			g.NewCard(triggerWatcherDef(t, "Test Lonely Watcher",
				"Mode$ Phase | Phase$ Upkeep | ValidPlayer$ You | IsPresent$ Creature.YouCtrl | PresentCompare$ EQ0 | OptionalDecider$ You | TriggerZones$ Battlefield | Execute$ TrigGain"+tc.extra,
				"TrigGain", "DB$ GainLife | Defined$ You | LifeAmount$ 3"), p, engine.Battlefield)
			g.SetTurnState(2, p, engine.Untap)
			c := engine.NewScriptedController()
			g.AdvancePhase(c)
			if got := g.StackLen(); got != 1 {
				t.Fatalf("StackLen() = %d, want the upkeep trigger", got)
			}
			if tc.creatureCame {
				g.NewCard(creatureDefPT(t, "2", "2"), p, engine.Battlefield)
			}
			if tc.want > 20 {
				c.QueueConfirmOptionalTrigger(true)
			}
			if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
				t.Fatalf("ResolveStack: %v", err)
			}
			if got := g.Player(p).Life; got != tc.want {
				t.Errorf("life = %d, want %d", got, tc.want)
			}
		})
	}
}
