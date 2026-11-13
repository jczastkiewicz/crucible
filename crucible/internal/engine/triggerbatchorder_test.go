package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
)

// CR 603.3b over modes: every trigger one event raises -- whatever its mode --
// goes on the stack in one APNAP pass, the active player's first (lowest), the
// non-active player's last (on top, resolving first). Java collects every mode
// into MagicStack's simultaneous list before ordering it per player
// (MagicStack.java:855-880). The non-active player's trigger here is of the
// mode the engine checks first, so ordering per mode would leave it under the
// active player's trigger of a later mode.
func TestTriggersOfDifferentModesShareOneAPNAPPass(t *testing.T) {
	t.Parallel()

	t.Run("declare blockers", func(t *testing.T) {
		t.Parallel()

		g, p, other, attacker := attackingCombat(t)
		blocker := g.NewCard(creatureDefPT(t, "2", "2"), p, engine.Battlefield)
		// p (non-active): Mode$ Blocks, checked before AttackerBlocked.
		g.NewCard(watcherDef(t, "Blocks Watcher", "Mode$ Blocks | ValidCard$ Creature | TriggerZones$ Battlefield",
			"DB$ GainLife | Defined$ You | LifeAmount$ 1"), p, engine.Battlefield)
		// other (active): Mode$ AttackerBlocked.
		g.NewCard(watcherDef(t, "Blocked Watcher", "Mode$ AttackerBlocked | ValidCard$ Creature | TriggerZones$ Battlefield",
			"DB$ GainLife | Defined$ You | LifeAmount$ 1"), other, engine.Battlefield)
		g.SetTurnState(1, other, engine.DeclareBlockers)
		bc := engine.NewScriptedController()
		bc.QueueBlocks([]engine.Block{{Blocker: blocker, Attacker: attacker}})
		declareBlockers(t, g, bc)

		if got := g.StackLen(); got != 2 {
			t.Fatalf("stack = %d, want both watchers' triggers", got)
		}
		if top, _ := g.StackTop(); top.Controller != p {
			t.Errorf("top of the stack is %v's trigger, want the non-active player's (%v)", top.Controller, p)
		}
	})

	t.Run("declare attackers", func(t *testing.T) {
		t.Parallel()

		g, p, other := newTwoPlayerGameOn(t, scenarioDB(t))
		g.SetTurnState(1, other, engine.DeclareAttackers)
		attacker := g.NewCard(creatureDefPT(t, "2", "2"), other, engine.Battlefield)
		// p (non-active): Mode$ Attacks, checked before AttackersDeclared.
		g.NewCard(watcherDef(t, "Attacks Watcher", "Mode$ Attacks | ValidCard$ Creature | TriggerZones$ Battlefield",
			"DB$ GainLife | Defined$ You | LifeAmount$ 1"), p, engine.Battlefield)
		// other (active): Mode$ AttackersDeclared.
		g.NewCard(watcherDef(t, "Declared Watcher", "Mode$ AttackersDeclared | TriggerZones$ Battlefield",
			"DB$ GainLife | Defined$ You | LifeAmount$ 1"), other, engine.Battlefield)
		ac := engine.NewScriptedController()
		ac.QueueAttackers([]engine.CardID{attacker})
		declareAttackers(t, g, ac)

		if got := g.StackLen(); got != 2 {
			t.Fatalf("stack = %d, want both watchers' triggers", got)
		}
		if top, _ := g.StackTop(); top.Controller != p {
			t.Errorf("top of the stack is %v's trigger, want the non-active player's (%v)", top.Controller, p)
		}
	})
}

// CR 603.3b: a player with several triggers that fire together orders them
// (PlayerController.orderSimultaneousSa); with no answer the first found goes
// on the stack first and so resolves last.
func TestPlayerOrdersTheirSimultaneousTriggers(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name  string
		order []int
		want  int // index of the watcher expected on top of the stack
	}{
		{"default", nil, 1},
		{"first resolves first", []int{0, 1}, 0},
		{"second resolves first", []int{1, 0}, 1},
		{"not a permutation is ignored", []int{0, 0}, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			g, _, other := newTwoPlayerGameOn(t, scenarioDB(t))
			g.SetTurnState(1, other, engine.DeclareAttackers)
			attacker := g.NewCard(creatureDefPT(t, "2", "2"), other, engine.Battlefield)
			var watchers [2]engine.CardID
			for i := range watchers {
				watchers[i] = g.NewCard(watcherDef(t, "Attacks Watcher", "Mode$ Attacks | ValidCard$ Creature | TriggerZones$ Battlefield",
					"DB$ GainLife | Defined$ You | LifeAmount$ 1"), other, engine.Battlefield)
			}
			ac := engine.NewScriptedController()
			ac.QueueAttackers([]engine.CardID{attacker})
			if tc.order != nil {
				ac.QueueTriggerOrder(tc.order)
			}
			declareAttackers(t, g, ac)

			if got := g.StackLen(); got != 2 {
				t.Fatalf("stack = %d, want both watchers' triggers", got)
			}
			if top, _ := g.StackTop(); top.Source != watchers[tc.want] {
				t.Errorf("top of the stack is %v, want watcher %d (%v)", top.Source, tc.want, watchers[tc.want])
			}
		})
	}
}

// triggerOrderReader records the triggers a player is asked to order.
type triggerOrderReader struct {
	*engine.ScriptedController
	items []engine.TriggerOrderItem
}

func (r *triggerOrderReader) OrderSimultaneousTriggers(g *engine.Game, decider engine.PlayerID, items []engine.TriggerOrderItem) []int {
	r.items = items
	return r.ScriptedController.OrderSimultaneousTriggers(g, decider, items)
}

// Two triggers of one card differ by the effect they run, which the player
// ordering them is told (Haliya's draw beside its warp exile).
func TestTriggersToOrderNameTheirEffect(t *testing.T) {
	t.Parallel()

	g, _, other := newTwoPlayerGameOn(t, scenarioDB(t))
	g.SetTurnState(1, other, engine.DeclareAttackers)
	attacker := g.NewCard(creatureDefPT(t, "2", "2"), other, engine.Battlefield)
	watcher := g.NewCard(scriptDef(t, "Twin Watcher", "Artifact",
		"T:Mode$ Attacks | ValidCard$ Creature | TriggerZones$ Battlefield | Execute$ TrigLife",
		"T:Mode$ Attacks | ValidCard$ Creature | TriggerZones$ Battlefield | Execute$ TrigDraw",
		"SVar:TrigLife:DB$ GainLife | Defined$ You | LifeAmount$ 1",
		"SVar:TrigDraw:DB$ Draw | Defined$ You | NumCards$ 1"), other, engine.Battlefield)
	r := &triggerOrderReader{ScriptedController: engine.NewScriptedController()}
	r.QueueAttackers([]engine.CardID{attacker})
	declareAttackers(t, g, r)

	got := map[string]bool{}
	for _, it := range r.items {
		if it.Source == watcher {
			got[it.API] = true
		}
	}
	if len(r.items) != 2 || !got["GainLife"] || !got["Draw"] {
		t.Errorf("asked to order %+v, want the watcher's GainLife and Draw", r.items)
	}
}
