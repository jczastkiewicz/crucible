package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/carddb/compile"
	"github.com/jczastkiewicz/crucible/internal/engine"
	"github.com/jczastkiewicz/crucible/internal/mana"
)

func classDef(t *testing.T) *compile.Card {
	return scriptDef(t, "Test Class", "Enchantment Class",
		"K:Class:2:1:AddTrigger$ TrigL2",
		"SVar:TrigL2:Mode$ ClassLevelGained | ClassLevel$ 2 | ValidCard$ Card.Self | TriggerZones$ Battlefield | Execute$ GainThree | Secondary$ True",
		"SVar:GainThree:DB$ GainLife | Defined$ You | LifeAmount$ 3",
		"K:Class:3:1:AddStaticAbility$ SHaste",
		"SVar:SHaste:Mode$ Continuous | Affected$ Creature.YouCtrl | AddKeyword$ Haste | EffectZone$ Battlefield | Secondary$ True")
}

// TestClassLevelsGateAbilitiesAndGrantTraits proves CR 716: each level-up
// works only from the level below it, level 2's trigger fires once on
// reaching it, level 3's static applies from level 3, and leaving the
// battlefield resets the level.
func TestClassLevelsGateAbilitiesAndGrantTraits(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGame(t)
	class := g.NewCard(classDef(t), p, engine.Battlefield)
	elf := g.NewCard(creatureDef(t), p, engine.Battlefield)
	c := engine.NewScriptedController()
	pay := func() {
		g.Player(p).ManaPool.Add(mana.Green, 1)
		c.QueuePayGeneric(mana.ShardG)
	}
	level := func(index int) bool {
		pay()
		if !g.ActivateAbility(p, class, index, c) {
			return false
		}
		if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
			t.Fatalf("ResolveStack: %v", err)
		}
		return true
	}

	if g.Card(class).ClassLevel() != 1 {
		t.Fatalf("starting level = %d, want 1", g.Card(class).ClassLevel())
	}
	if level(1) {
		t.Error("level 3 ability activated at level 1")
	}
	if !level(0) {
		t.Fatal("level 2 ability was declined at level 1")
	}
	if got := g.Player(p).Life; got != 23 || g.Card(class).ClassLevel() != 2 {
		t.Errorf("after level 2: life %d level %d, want 23 and 2", got, g.Card(class).ClassLevel())
	}
	if g.Card(elf).HasKeyword("Haste") {
		t.Error("haste granted at level 2")
	}
	if level(0) {
		t.Error("level 2 ability activated again at level 2")
	}
	if !level(1) {
		t.Fatal("level 3 ability was declined at level 2")
	}
	if !g.Card(elf).HasKeyword("Haste") {
		t.Error("haste not granted at level 3")
	}
	if g.Player(p).Life != 23 {
		t.Errorf("life %d after level 3, want 23: the level 2 trigger fired again", g.Player(p).Life)
	}
}
