package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
)

// TestGrantedAbilityReadsTheSVarAddSVarNames proves AddSVar$ on a granted
// body (Sword of Fire and Ice's "AddSVar$ SwordOfFireAndIceCE" family): the
// receiving creature's granted ability counts through an SVar the granting
// card defines, here a numeric one (Count$Valid Creature.YouCtrl), because a
// grant's amounts are the granting face's. A creature the grant does not
// reach has no such ability.
func TestGrantedAbilityReadsTheSVarAddSVarNames(t *testing.T) {
	t.Parallel()

	g, p := textGame(t)
	g.NewCard(scriptDef(t, "Granter", "Enchantment",
		"S:Mode$ Continuous | Affected$ Creature.YouCtrl | AddAbility$ ABGrow | AddSVar$ GrowX",
		"SVar:ABGrow:AB$ Pump | Cost$ 0 | Defined$ Self | NumAtt$ GrowX",
		"SVar:GrowX:Count$Valid Creature.YouCtrl"), p, engine.Battlefield)
	bears := g.NewCard(creatureDefPT(t, "2", "2"), p, engine.Battlefield)
	g.NewCard(creatureDefPT(t, "1", "1"), p, engine.Battlefield)
	sba(g)

	c := engine.NewScriptedController()
	if !g.ActivateAbility(p, bears, 0, c) {
		t.Fatal("ActivateAbility(granted ability) = false")
	}
	if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
		t.Fatal(err)
	}
	sba(g)
	wantPT(t, g, bears, 4, 2) // +2: two creatures
}
