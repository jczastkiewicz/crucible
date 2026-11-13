package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
	"github.com/jczastkiewicz/crucible/internal/mana"
)

// The stack is frozen while an activated ability's costs are paid, its mana
// included (PlaySpellAbility.java:682): a creature sacrificed for the mana
// (an Eldrazi Spawn paying an equip) triggers above the ability, so the
// trigger resolves first.
func TestManaSacrificedForAnActivationTriggersAboveIt(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGameOn(t, scenarioDB(t))
	g.SetTurnState(1, p, engine.Main1)
	spawn := g.NewCard(scriptDef(t, "Test Spawn", "Creature Eldrazi Spawn",
		"A:AB$ Mana | Cost$ Sac<1/CARDNAME> | Produced$ C"), p, engine.Battlefield)
	g.NewCard(scriptDef(t, "Death Watcher", "Artifact",
		"T:Mode$ ChangesZone | Origin$ Battlefield | Destination$ Graveyard | ValidCard$ Creature | TriggerZones$ Battlefield | Execute$ TrigLife",
		"SVar:TrigLife:DB$ GainLife | Defined$ You | LifeAmount$ 1"), p, engine.Battlefield)
	shrine := g.NewCard(scriptDef(t, "Test Shrine", "Artifact",
		"A:AB$ Draw | Cost$ 1 | Defined$ You | NumCards$ 1"), p, engine.Battlefield)
	c := engine.NewScriptedController()
	c.QueuePayManaCost([]engine.Action{{Kind: engine.ActionManaAbility, Card: spawn}})
	c.QueuePayGeneric(mana.ShardC)
	if !g.ActivateAbility(p, shrine, 0, c) {
		t.Fatalf("shrine not activated: %v", g.TakePendingError())
	}
	if g.Card(spawn).Zone != engine.Graveyard {
		t.Fatalf("spawn in %v, want it sacrificed for the mana", g.Card(spawn).Zone)
	}
	if top, ok := g.StackTop(); !ok || top.API != engine.APIGainLife || g.StackLen() != 2 {
		t.Errorf("stack len %d, top %v; want the dies trigger above the draw", g.StackLen(), top.API)
	}
	top, _ := g.StackTop()
	if src, spell, ok := g.StackItemOf(top.ID); !ok || spell || g.Card(src).Name() != "Death Watcher" {
		t.Errorf("top item of %v, spell %v, found %v; want the watcher's trigger", src, spell, ok)
	}
}
