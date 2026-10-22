package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/carddb"
	"github.com/jczastkiewicz/crucible/internal/carddb/compile"
	"github.com/jczastkiewicz/crucible/internal/cardtype"
	"github.com/jczastkiewicz/crucible/internal/engine"
)

// torporOrbDef is a permanent with Torpor Orb's static.
func torporOrbDef(t *testing.T) *compile.Card {
	t.Helper()
	raw := &carddb.Card{Filename: "Torpor Orb"}
	raw.Faces[0].Present = true
	raw.Faces[0].Name = "Torpor Orb"
	raw.Faces[0].Type = cardtype.Parse(attachmentTypeRegistry(t), "Artifact")
	raw.Faces[0].Statics = []string{"Mode$ DisableTriggers | ValidCause$ Creature | ValidMode$ ChangesZone,ChangesZoneAll | Destination$ Battlefield"}
	c, err := compile.Compile(raw)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	return c
}

// TestDisableTriggersStopsAnEntersTriggerWhileTheStaticIsInPlay proves
// StaticAbilityDisableTriggers (Torpor Orb) keeps a creature's enters
// trigger off the stack, and that the trigger fires again once the static
// leaves.
func TestDisableTriggersStopsAnEntersTriggerWhileTheStaticIsInPlay(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGame(t)
	orb := g.NewCard(torporOrbDef(t), p, engine.Battlefield)

	cast := func() int {
		before := g.Player(p).Life
		if _, err := castETBChain(t, g, p, etbChainDef(t, "Greeter", "DB$ GainLife | Defined$ You | LifeAmount$ 3"), engine.NewScriptedController()); err != nil {
			t.Fatalf("cast: %v", err)
		}
		return g.Player(p).Life - before
	}
	if got := cast(); got != 0 {
		t.Errorf("with the static: gained %d, want 0", got)
	}
	g.Move(orb, engine.Exile, p)
	if got := cast(); got != 3 {
		t.Errorf("without the static: gained %d, want 3", got)
	}
}
