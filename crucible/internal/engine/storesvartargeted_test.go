package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
	"github.com/jczastkiewicz/crucible/internal/mana"
)

// StoreSVar Type$ Targeted (Drain Life, Soul Burn) measures the targeted
// cards with Expression$ (StoreSVarEffect.java:55-57): a 1/4 creature stores
// a Limit of 4, which the next ability reads.
func TestStoreSVarTargetedMeasuresTheTargetedCard(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGame(t)
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).ManaPool.Add(mana.Red, 1)
	wall := g.NewCard(creatureDefPT(t, "1", "4"), p, engine.Battlefield)
	def := scriptDef(t, "Test Store", "Sorcery",
		"A:SP$ StoreSVar | ValidTgts$ Creature | SVar$ Limit | Type$ Targeted | Expression$ CardToughness | SubAbility$ DBGain",
		"SVar:DBGain:DB$ GainLife | Defined$ You | LifeAmount$ Limit")
	def.Faces[0].ManaCost = mana.MustParse("R")
	spell := g.NewCard(def, p, engine.Hand)
	before := g.Player(p).Life
	c := engine.NewScriptedController()
	c.QueueTargets([]engine.EntityID{engine.CardEntity(wall)})
	if !g.CastSpell(p, spell, c) {
		t.Fatal("cast refused")
	}
	if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if got := g.Player(p).Life - before; got != 4 {
		t.Errorf("life gained = %d, want the target's toughness 4", got)
	}
}
