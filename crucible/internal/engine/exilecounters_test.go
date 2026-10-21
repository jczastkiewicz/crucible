package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
)

// TestExileCountersOnHiddenChangeZoneAndDig proves the counters a card gets
// as it is exiled: a hidden-origin ChangeZone adds WithCountersType$ after the
// move (ChangeZoneEffect.java:1508-1514), and Dig adds one ExileWithCounters$
// counter per exiled card (DigEffect.java:414, :490).
func TestExileCountersOnHiddenChangeZoneAndDig(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name, line string
		hand       bool
		want       int
	}{
		{"hidden ChangeZone amount", "DB$ ChangeZone | Hidden$ True | Origin$ Hand | Destination$ Exile | ChangeType$ Card | WithCountersType$ TIME | WithCountersAmount$ 3", true, 3},
		{"hidden ChangeZone default amount", "DB$ ChangeZone | Hidden$ True | Origin$ Hand | Destination$ Exile | ChangeType$ Card | WithCountersType$ TIME", true, 1},
		{"Dig ExileWithCounters", "DB$ Dig | DigNum$ 1 | DestinationZone$ Exile | SkipReorder$ True | ExileWithCounters$ TIME", false, 1},
		{"Dig without the param", "DB$ Dig | DigNum$ 1 | DestinationZone$ Exile | SkipReorder$ True", false, 0},
	} {
		g, p, other := newTwoPlayerGame(t)
		libraryCards(t, g, p, 1)
		libraryCards(t, g, other, 1)
		c := engine.NewScriptedController()
		if tc.hand {
			held := g.NewCard(creatureDefPT(t, "1", "1"), p, engine.Hand)
			c.QueueCardChoice([]engine.CardID{held})
		} else {
			c.QueueCardChoice(g.Zone(engine.Library, p).Cards()[:1])
		}
		def := etbChainDef(t, "Test Exile Counters", tc.line)
		if _, err := castETBChain(t, g, p, def, c); err != nil {
			t.Fatalf("%s: ResolveStack: %v", tc.name, err)
		}
		exiled := g.Zone(engine.Exile, p).Cards()
		if len(exiled) != 1 {
			t.Fatalf("%s: exiled %d cards, want 1", tc.name, len(exiled))
		}
		if got := g.Card(exiled[0]).Counters.Count(engine.CounterType("TIME")); got != tc.want {
			t.Errorf("%s: TIME counters = %d, want %d", tc.name, got, tc.want)
		}
	}
}
