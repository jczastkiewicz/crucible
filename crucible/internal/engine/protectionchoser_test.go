package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
)

// TestProtectionChoserControllerAsksTheTargetsController proves Choser$
// Controller (Pale Wayfarer): the target's controller picks the protection.
func TestProtectionChoserControllerAsksTheTargetsController(t *testing.T) {
	t.Parallel()

	g, p, other := newTwoPlayerGame(t)
	theirs := g.NewCard(creatureDefPT(t, "1", "1"), other, engine.Battlefield)
	rec := &protectionDeciderRecorder{ScriptedController: engine.NewScriptedController()}
	rec.QueueProtectionChoice(0)
	line := "DB$ Protection | Defined$ Targeted | Gains$ Choice | Choices$ AnyColor | Choser$ Controller | ValidTgts$ Creature"
	pushPlayLine(t, g, p, []engine.EntityID{engine.EntityID(theirs)}, line)
	if err := g.ResolveStack(engine.NewRegistry(), rec); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if rec.decider != other {
		t.Errorf("protection chosen by %v, want %v (the target's controller)", rec.decider, other)
	}
}

type protectionDeciderRecorder struct {
	*engine.ScriptedController
	decider engine.PlayerID
}

func (r *protectionDeciderRecorder) ChooseProtectionType(g *engine.Game, decider engine.PlayerID, src engine.CardID, choices []string) int {
	r.decider = decider
	return r.ScriptedController.ChooseProtectionType(g, decider, src, choices)
}
