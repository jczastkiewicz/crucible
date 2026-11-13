package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
)

// CR 603.3, 704.3 (ADR-0065): the triggers a step's turn-based actions fire
// wait for the state-based actions. A creature the combat damage kills
// triggers in the same APNAP pass as the damage, so its controller orders
// both at once; and lethal damage ends the game before either reaches the
// stack (PhaseHandler.java:1308-1318).
func TestCombatDamageTriggersWaitForTheStateBasedActions(t *testing.T) {
	t.Parallel()

	setup := func(t *testing.T, defenderLife int) (*engine.Game, engine.PlayerID, engine.PlayerID, engine.CardID) {
		g, p, other := newTwoPlayerGameOn(t, scenarioDB(t))
		g.Player(p).Life = defenderLife
		g.SetTurnState(1, other, engine.DeclareAttackers)
		attacker := g.NewCard(scriptDef(t, "Doomed Raider", "Creature Elf",
			"T:Mode$ DamageDone | ValidSource$ Card.Self | CombatDamage$ True | Execute$ TrigLife",
			"T:Mode$ ChangesZone | Origin$ Battlefield | Destination$ Graveyard | ValidCard$ Card.Self | Execute$ TrigLife",
			"SVar:TrigLife:DB$ GainLife | Defined$ You | LifeAmount$ 1"), other, engine.Battlefield)
		ac := engine.NewScriptedController()
		ac.QueueAttackers([]engine.CardID{attacker})
		declareAttackers(t, g, ac)
		g.SetTurnState(1, other, engine.DeclareBlockers)
		return g, p, other, attacker
	}
	// toDamage steps through first strike damage, which deals none here,
	// and the combat damage step.
	toDamage := func(t *testing.T, g *engine.Game, c engine.PlayerController) {
		for g.ActivePhase() != engine.CombatDamage && !g.Over() {
			if err := g.Step(engine.NewRegistry(), c); err != nil {
				t.Fatalf("Step: %v", err)
			}
		}
	}

	t.Run("dies in the same pass", func(t *testing.T) {
		t.Parallel()
		g, p, _, attacker := setup(t, 20)
		blocker := g.NewCard(creatureDefPT(t, "2", "2"), p, engine.Battlefield)
		bc := engine.NewScriptedController()
		bc.QueueBlocks([]engine.Block{{Blocker: blocker, Attacker: attacker}})
		declareBlockers(t, g, bc)
		r := &triggerOrderReader{ScriptedController: engine.NewScriptedController()}
		toDamage(t, g, r)
		if g.Card(attacker).Zone != engine.Graveyard {
			t.Fatalf("attacker in %v, want it dead", g.Card(attacker).Zone)
		}
		if len(r.items) != 2 {
			t.Errorf("asked to order %+v, want the damage and the death trigger together", r.items)
		}
	})

	t.Run("lethal damage pushes nothing", func(t *testing.T) {
		t.Parallel()
		g, p, other, _ := setup(t, 1)
		toDamage(t, g, engine.NewScriptedController())
		if !g.Over() || !g.Player(p).Lost {
			t.Fatalf("game over %v, defender lost %v; want the damage to end it", g.Over(), g.Player(p).Lost)
		}
		if got := g.StackLen(); got != 0 {
			t.Errorf("stack holds %d, want the damage trigger never pushed", got)
		}
		if got := g.Player(other).Life; got != 20 {
			t.Errorf("attacker's controller at %d life, want 20", got)
		}
	})
}
