package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
)

// ExileOnLost$ adds a LosesGame trigger to an effect card whose loser owns and
// controls it, so it never fires (EffectEffect.java:257-259, Game.java:897-902):
// the line resolves and does nothing.
func TestEffectExileOnLostResolvesAsANoOp(t *testing.T) {
	t.Parallel()
	g, p, _ := newTwoPlayerGame(t)
	if _, err := resolveNow(t, g, p, engine.NewScriptedController(), nil,
		"DB$ Effect | ReplacementEffects$ DMPrevent | ExileOnLost$ True | Duration$ Permanent",
		"DMPrevent", "Event$ DamageDone | Prevent$ True | ValidTarget$ You"); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
}

// WithCountersType$ on a hidden-origin ChangeZone is read for a battlefield or
// exile destination alone (ChangeZoneEffect.java:1331-1338, :1508-1514): a
// card put into the graveyard gets none.
func TestHiddenChangeZoneToGraveyardDropsWithCountersType(t *testing.T) {
	t.Parallel()
	g, p, _ := newTwoPlayerGame(t)
	lib := libraryCards(t, g, p, 3)
	c := engine.NewScriptedController()
	c.QueueCardChoice(lib[:1])
	if _, err := resolveNow(t, g, p, c, nil,
		"DB$ ChangeZone | Origin$ Library | Destination$ Graveyard | ChangeType$ Card | ChangeNum$ 1 | Mandatory$ True | WithCountersType$ P1P1"); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if g.Card(lib[0]).Zone != engine.Graveyard {
		t.Fatalf("card is in %v, want Graveyard", g.Card(lib[0]).Zone)
	}
	if n := g.Card(lib[0]).Counters.Count(engine.P1P1); n != 0 {
		t.Errorf("P1P1 = %d, want 0", n)
	}
}
