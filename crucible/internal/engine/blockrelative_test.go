package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
)

// ironclawGame seats Ironclaw Curse on a Hill Giant (3/3, 3/2 enchanted) for
// the blocking side and returns the giant, plus attackers of power 1, 2 and 3
// for the attacking side.
func ironclawGame(t *testing.T) (g *engine.Game, giant engine.CardID, attackers [3]engine.CardID) {
	t.Helper()
	g, p, other := newTwoPlayerGameOn(t, scenarioDB(t))
	g.SetTurnState(1, p, engine.Main1)
	giant = g.NewCard(corpusCard(t, "Hill Giant"), other, engine.Battlefield)
	aura := g.NewCard(corpusCard(t, "Ironclaw Curse"), other, engine.Battlefield)
	g.Attach(aura, giant)
	attackers[0] = g.NewCard(corpusCard(t, "Llanowar Elves"), p, engine.Battlefield)
	attackers[1] = g.NewCard(corpusCard(t, "Grizzly Bears"), p, engine.Battlefield)
	attackers[2] = g.NewCard(corpusCard(t, "Hill Giant"), p, engine.Battlefield)
	engine.CheckStateBasedActions(g, engine.NewScriptedController())
	return g, giant, attackers
}

// TestIronclawCurseRefusesBlocksAtOrAboveTheEnchantedToughness pins
// ValidAttackerRelative$ Creature.powerGEIronclawX (StaticAbilityCantAttack
// Block.java:263): the enchanted creature (toughness 3 - 1 = 2) cannot block
// an attacker with power 2 or more, and can block a power-1 one.
func TestIronclawCurseRefusesBlocksAtOrAboveTheEnchantedToughness(t *testing.T) {
	t.Parallel()

	g, giant, attackers := ironclawGame(t)
	for i, want := range []bool{true, false, false} {
		if got := g.CanBlock(attackers[i], giant); got != want {
			t.Errorf("CanBlock(attacker of power %d, enchanted Hill Giant) = %v, want %v", i+1, got, want)
		}
	}
}

// TestIronclawCurseLeavesOtherCreaturesFree: the static restricts only
// ValidBlocker$ Creature.EnchantedBy, so an unenchanted creature blocks
// anything. An absent ValidAttacker$ matches every attacker.
func TestIronclawCurseLeavesOtherCreaturesFree(t *testing.T) {
	t.Parallel()

	g, _, attackers := ironclawGame(t)
	free := g.NewCard(corpusCard(t, "Hill Giant"), g.Players()[1], engine.Battlefield)
	for i, attacker := range attackers {
		if !g.CanBlock(attacker, free) {
			t.Errorf("an unenchanted creature cannot block the power-%d attacker", i+1)
		}
	}
}

// TestStarToughnessWithNoCharacteristicDefiningAbilityIsZero pins CR 704.5f
// for a printed "*": CardFace.parsePT reads it as 0 ("1+*" as 1), so a
// creature whose characteristic-defining ability does not apply dies at "*"
// and survives at "1+*".
func TestStarToughnessWithNoCharacteristicDefiningAbilityIsZero(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGame(t)
	g.SetTurnState(1, p, engine.Main1)
	star := g.NewCard(creatureDefPT(t, "*", "*"), p, engine.Battlefield)
	plusOne := g.NewCard(creatureDefPT(t, "*", "1+*"), p, engine.Battlefield)
	if got, ok := g.Card(plusOne).Toughness(); !ok || got != 1 {
		t.Errorf("Toughness of */1+* = %d, %v, want 1, true", got, ok)
	}
	engine.CheckStateBasedActions(g, engine.NewScriptedController())
	if z := g.Card(star).Zone; z != engine.Graveyard {
		t.Errorf("*/* zone = %v, want Graveyard (toughness 0)", z)
	}
	if z := g.Card(plusOne).Zone; z != engine.Battlefield {
		t.Errorf("*/1+* zone = %v, want Battlefield (toughness 1)", z)
	}
}

// TestToughnessReducedByACounterKills: a -1/-1 counter on a 1/1 reaches
// 704.5f through Card.Toughness.
func TestToughnessReducedByACounterKills(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGame(t)
	g.SetTurnState(1, p, engine.Main1)
	id := g.NewCard(creatureDefPT(t, "1", "1"), p, engine.Battlefield)
	g.Card(id).Counters.Add(engine.M1M1, 1)
	engine.CheckStateBasedActions(g, engine.NewScriptedController())
	if z := g.Card(id).Zone; z != engine.Graveyard {
		t.Errorf("1/1 with a -1/-1 counter: zone = %v, want Graveyard", z)
	}
}
