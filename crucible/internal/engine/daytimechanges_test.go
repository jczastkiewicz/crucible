package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
)

// TestDayTimeChangesFiresOnlyWhenTheValueFlips proves Game.setDayTime
// (Game.java:1423-1433): neither-to-day is silent, day-to-night fires, and
// setting the value it already has is silent.
func TestDayTimeChangesFiresOnlyWhenTheValueFlips(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGame(t)
	g.NewCard(planarDef(t, "Dusk Watcher", "Creature Elf", []string{
		"Mode$ DayTimeChanges | TriggerZones$ Battlefield | Execute$ Plus",
	}, nil, "Plus", "DB$ GainLife | Defined$ You | LifeAmount$ 1"), p, engine.Battlefield)

	steps := []struct {
		line string
		gain int
	}{
		{"DB$ DayTime | Value$ Day", 0},
		{"DB$ DayTime | Value$ Day", 0},
		{"DB$ DayTime | Value$ Night", 1},
		{"DB$ DayTime | Value$ Switch", 1},
	}
	ctl := engine.NewScriptedController()
	for _, s := range steps {
		before := g.Player(p).Life
		resolveLine(t, g, p, ctl, s.line)
		if err := g.ResolveStack(engine.NewRegistry(), ctl); err != nil {
			t.Fatalf("ResolveStack: %v", err)
		}
		if got := g.Player(p).Life - before; got != s.gain {
			t.Errorf("%s: gained %d, want %d", s.line, got, s.gain)
		}
	}
}
