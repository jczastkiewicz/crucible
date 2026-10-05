package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
)

// A player with Protection from a Curse's characteristic refuses it
// (PlayerFactoryUtil.java:42-52, the CantAttach half of CR 702.16c): the Curse
// attached to them falls off as a state-based action. A Protection line that
// does not match leaves it. The Protection comes from a continuous static
// (Gor Muldrak's shape), since state-based actions rebuild the continuous
// effects.
func TestPlayerProtectionRefusesAnAttachedCurse(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name       string
		protection string
		want       engine.ZoneType
	}{
		{"matching Protection", "Protection:Enchantment", engine.Graveyard},
		{"another characteristic", "Protection:Artifact", engine.Battlefield},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			g, p, opp := newTwoPlayerGameOn(t, scenarioDB(t))
			g.NewCard(scriptDef(t, "Test Ward Stone", "Artifact",
				"S:Mode$ Continuous | Affected$ You | AddKeyword$ "+tc.protection), opp, engine.Battlefield)
			curse := g.NewCard(scriptDef(t, "Test Curse", "Enchantment Aura Curse", "K:Enchant:Player"), p, engine.Battlefield)
			g.AttachToPlayer(curse, opp)
			engine.CheckStateBasedActions(g, engine.NewScriptedController())
			if got := g.Card(curse).Zone; got != tc.want {
				t.Errorf("Curse zone = %v, want %v", got, tc.want)
			}
		})
	}
}
