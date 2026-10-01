package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
)

// unblockedHit runs an unblocked attack by attacker (a creature of a's) and
// returns after combat damage.
func unblockedHit(t *testing.T, g *engine.Game, attacker engine.CardID) {
	t.Helper()
	declareAttacking(t, g, attacker)
	bc := engine.NewScriptedController()
	bc.QueueBlocks(nil)
	declareBlockers(t, g, bc)
	g.DealCombatDamage(engine.NewScriptedController())
}

// CR 120.3b, 702.90b: infect damage to a player is that many poison counters
// and no life loss.
func TestInfectDamageToAPlayerIsPoisonCounters(t *testing.T) {
	t.Parallel()

	g, a, b := combatGame(t)
	unblockedHit(t, g, g.NewCard(creatureDefPTKeywords(t, "3", "3", "Infect"), a, engine.Battlefield))

	if got := g.Player(b).Life; got != 20 {
		t.Errorf("defender life = %d, want 20 -- infect deals no life loss", got)
	}
	if got := g.Player(b).Counters.Count(engine.Poison); got != 3 {
		t.Errorf("poison counters = %d, want 3", got)
	}
}

// CR 702.164c: toxic N adds N poison counters to combat damage, and normal
// damage still costs life.
func TestToxicAddsPoisonCountersToCombatDamage(t *testing.T) {
	t.Parallel()

	g, a, b := combatGame(t)
	unblockedHit(t, g, g.NewCard(creatureDefPTKeywords(t, "3", "3", "Toxic:2"), a, engine.Battlefield))

	if got := g.Player(b).Life; got != 17 {
		t.Errorf("defender life = %d, want 17", got)
	}
	if got := g.Player(b).Counters.Count(engine.Poison); got != 2 {
		t.Errorf("poison counters = %d, want 2", got)
	}
}

// CR 120.3d: wither and infect damage to a creature is -1/-1 counters, not
// marked damage, and the damage still counts as dealt for lifelink.
func TestWitherAndInfectDamageToACreatureAreMinusCounters(t *testing.T) {
	t.Parallel()

	for _, kw := range []string{"Wither", "Infect"} {
		t.Run(kw, func(t *testing.T) {
			t.Parallel()

			g, a, b := combatGame(t)
			attacker := g.NewCard(creatureDefPTKeywords(t, "2", "2", kw, "Lifelink"), a, engine.Battlefield)
			blocker := g.NewCard(creatureDefPT(t, "1", "5"), b, engine.Battlefield)
			declareAttacking(t, g, attacker)
			bc := engine.NewScriptedController()
			bc.QueueBlocks([]engine.Block{{Blocker: blocker, Attacker: attacker}})
			declareBlockers(t, g, bc)
			g.DealCombatDamage(engine.NewScriptedController())

			c := g.Card(blocker)
			if c.Damage.Marked != 0 || c.Counters.Count(engine.M1M1) != 2 {
				t.Errorf("marked damage %d, -1/-1 counters %d, want 0 and 2", c.Damage.Marked, c.Counters.Count(engine.M1M1))
			}
			if power, _ := c.Power(); power != -1 {
				t.Errorf("blocker power = %d, want -1 (1 minus two counters)", power)
			}
			if got := g.Player(a).Life; got != 22 {
				t.Errorf("attacker life = %d, want 22 -- lifelink counts the wither damage", got)
			}
		})
	}
}

// Deathtouch still sets its flag when the damage is dealt as counters (Card.addDamageAfterPrevention).
func TestWitherDeathtouchStillSetsTheFlag(t *testing.T) {
	t.Parallel()

	g, a, b := combatGame(t)
	attacker := g.NewCard(creatureDefPTKeywords(t, "1", "1", "Wither", "Deathtouch"), a, engine.Battlefield)
	blocker := g.NewCard(creatureDefPT(t, "1", "9"), b, engine.Battlefield)
	declareAttacking(t, g, attacker)
	bc := engine.NewScriptedController()
	bc.QueueBlocks([]engine.Block{{Blocker: blocker, Attacker: attacker}})
	declareBlockers(t, g, bc)
	g.DealCombatDamage(engine.NewScriptedController())

	if !g.Card(blocker).Damage.Deathtouch || g.Card(blocker).Damage.Marked != 0 {
		t.Errorf("deathtouch flag %v marked %d, want true and 0", g.Card(blocker).Damage.Deathtouch, g.Card(blocker).Damage.Marked)
	}
}
