package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
)

// ADR-0023: a Mode$ Continuous static's AddAbility$ gives each affected card
// the compiled ability (Cryptolith Rite's "creatures you control have '{T}: Add
// {G}'"), for as long as the static is on the battlefield.
func TestContinuousAddAbilityGrantsAManaAbility(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGame(t)
	rite := g.NewCard(scriptDef(t, "Test Rite", "Enchantment",
		"S:Mode$ Continuous | Affected$ Creature.YouCtrl | AddAbility$ ABMana",
		"SVar:ABMana:AB$ Mana | Cost$ T | Produced$ G"), p, engine.Battlefield)
	bears := g.NewCard(creatureDefPT(t, "2", "2"), p, engine.Battlefield)
	g.Card(bears).SummonSick = false
	sba(g)

	if !g.ActivateManaAbility(p, bears, 0, engine.NewScriptedController()) {
		t.Fatal("the granted mana ability could not be activated")
	}
	if got := g.Player(p).ManaPool.Total(); got != 1 {
		t.Errorf("mana in the pool = %d, want 1", got)
	}

	// The grant ends the pass its source leaves.
	g.Card(bears).Tapped = false
	g.Move(rite, engine.Graveyard, p)
	sba(g)
	if g.ActivateManaAbility(p, bears, 0, engine.NewScriptedController()) {
		t.Error("the ability outlived the static that granted it")
	}
}

// AddTrigger$ grants a trigger the same way: "creatures you control have
// 'whenever this creature attacks, you gain 1 life'".
func TestContinuousAddTriggerGrantsATrigger(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGame(t)
	g.NewCard(scriptDef(t, "Test Banner", "Enchantment",
		"S:Mode$ Continuous | Affected$ Creature.YouCtrl | AddTrigger$ TrigAttack",
		"SVar:TrigAttack:Mode$ Attacks | ValidCard$ Card.Self | Execute$ TrigGain | TriggerZones$ Battlefield",
		"SVar:TrigGain:DB$ GainLife | Defined$ You | LifeAmount$ 1"), p, engine.Battlefield)
	bears := g.NewCard(creatureDefPT(t, "2", "2"), p, engine.Battlefield)
	g.Card(bears).SummonSick = false
	sba(g)
	g.SetTurnState(1, p, engine.Main1)

	ac := engine.NewScriptedController()
	ac.QueueAttackers([]engine.CardID{bears})
	if _, err := g.DeclareCombatAttackers(ac); err != nil {
		t.Fatalf("DeclareCombatAttackers: %v", err)
	}
	if err := g.ResolveStack(engine.NewRegistry(), ac); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if got := g.Player(p).Life; got != 21 {
		t.Errorf("life = %d, want 21 -- the granted attack trigger should have fired", got)
	}
}
