package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
)

// Special Move's Foot Toss: "any other target" is Any.NotDefinedParentTarget,
// Player (CardProperty.java:2067-2071), so the link's candidates leave out the
// creature its parent targeted but keep every player.
func TestNotDefinedParentTargetExcludesTheParentsTarget(t *testing.T) {
	t.Parallel()

	g, p, other := newTwoPlayerGame(t)
	own := g.NewCard(creatureDefPT(t, "2", "2"), p, engine.Battlefield)
	theirs := g.NewCard(creatureDefPT(t, "2", "5"), other, engine.Battlefield)
	c := engine.NewScriptedController()
	c.QueueTargets([]engine.EntityID{engine.CardEntity(own)})
	c.QueueTargets([]engine.EntityID{engine.CardEntity(theirs)})
	resolveLine(t, g, p, c,
		"DB$ Pump | ValidTgts$ Creature.YouCtrl | SubAbility$ Hit",
		"Hit", "DB$ DealDamage | ValidTgts$ Any.NotDefinedParentTarget,Player | NumDmg$ 3 | DamageSource$ ParentTarget")
	if got := g.Card(theirs).Damage.Marked; got != 3 {
		t.Errorf("their creature has %d damage, want 3", got)
	}
}
