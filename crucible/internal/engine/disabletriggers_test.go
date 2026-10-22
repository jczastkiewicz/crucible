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

// TestDisableTriggersReachesTheExilePath proves the static also stops a
// creature's own leaves-the-battlefield trigger when it is exiled as a cost
// (Hushbringer-shaped Origin$ Battlefield, Destination$ Exile).
func TestDisableTriggersReachesTheExilePath(t *testing.T) {
	t.Parallel()

	const shieldLine = "S:Mode$ DisableTriggers | ValidCause$ Creature | ValidMode$ ChangesZone,ChangesZoneAll | Origin$ Battlefield | Destination$ Exile"
	for _, withStatic := range []bool{false, true} {
		g, p, _ := newTwoPlayerGame(t)
		g.NewCard(nil, p, engine.Library)
		if withStatic {
			g.NewCard(scriptDef(t, "Shield", "Artifact", shieldLine), p, engine.Battlefield)
		}
		def := creatureDefWithAbilityAndTrigger(t, "Exiler",
			"AB$ GainLife | Cost$ Exile<1/CARDNAME> | Defined$ You | LifeAmount$ 1",
			"Mode$ ChangesZone | Origin$ Battlefield | Destination$ Exile | ValidCard$ Card.Self | Execute$ TrigDraw")
		creature := g.NewCard(def, p, engine.Battlefield)
		if !g.ActivateAbility(p, creature, 0, engine.NewScriptedController()) {
			t.Fatal("ActivateAbility failed")
		}
		want := 2
		if withStatic {
			want = 1
		}
		if got := g.StackLen(); got != want {
			t.Errorf("static=%v: StackLen = %d, want %d", withStatic, got, want)
		}
	}
}
