package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
)

// GenericChoice AtRandom$ Urza (Urza, Academy Headmaster) replaces a drawn
// choice that targets with no candidate by a draw from the whole Choices$ list
// until it lands on one that can be taken (ChooseGenericEffect.java:91-100).
// With no Dragon in play the only choice that can resolve is the one that
// does not target, whatever the stream draws.
func TestGenericChoiceUrzaRedrawsTargetingChoicesWithNoCandidates(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGame(t)
	for i := range 20 {
		def := etbChainDef(t, "Test Urza", "DB$ GenericChoice | Defined$ You | AtRandom$ Urza | Choices$ DBA,DBB,DBC",
			"DBA", "DB$ Destroy | ValidTgts$ Creature.Dragon",
			"DBB", "DB$ GainLife | Defined$ You | LifeAmount$ 3",
			"DBC", "DB$ Pump | ValidTgts$ Creature.Dragon | NumAtt$ 1")
		if _, err := castETBChain(t, g, p, def, engine.NewScriptedController()); err != nil {
			t.Fatalf("ResolveStack: %v", err)
		}
		if got, want := g.Player(p).Life, 20+3*(i+1); got != want {
			t.Fatalf("after cast %d life = %d, want %d (only the untargeted choice can resolve)", i+1, got, want)
		}
	}
}
