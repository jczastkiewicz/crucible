package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
	"github.com/jczastkiewicz/crucible/internal/mana"
)

// A Panharmonicon line with no ValidMode$ covers every mode
// (StaticAbilityPanharmonicon.java:61-83): a SpellCast trigger of a permanent
// it matches goes on the stack twice, and a line naming an event param
// (ValidCause$) is not applied outside the sites that read the event.
func TestPanharmoniconDoublesAnyModeByHost(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, static string
		want         int
	}{
		{"no ValidMode", "Mode$ Panharmonicon | ValidCard$ Permanent.YouCtrl", 2},
		{"ValidMode names the mode", "Mode$ Panharmonicon | ValidMode$ SpellCast | ValidCard$ Permanent.YouCtrl", 2},
		{"ValidMode names another mode", "Mode$ Panharmonicon | ValidMode$ Drawn | ValidCard$ Permanent.YouCtrl", 1},
		{"ValidCard does not match the host", "Mode$ Panharmonicon | ValidCard$ Permanent.YouDontCtrl", 1},
		{"an event param is left to its own site", "Mode$ Panharmonicon | ValidMode$ SpellCast | ValidCause$ Instant | ValidCard$ Permanent.YouCtrl", 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			g, p, _ := newTwoPlayerGame(t)
			g.NewCard(continuousDef(t, "Doubler", tc.static), p, engine.Battlefield)
			g.NewCard(triggerWatcherDef(t, "Watcher", "Mode$ SpellCast | ValidActivatingPlayer$ You | Execute$ TrigGain", "TrigGain", "DB$ GainLife | Defined$ You | LifeAmount$ 1"), p, engine.Battlefield)
			spell := g.NewCard(gainInstant(t, "Gain", "W", "3"), p, engine.Hand)
			g.Player(p).ManaPool.Add(mana.White, 1)
			c := engine.NewScriptedController()
			before := g.Player(p).Life
			if !g.CastSpell(p, spell, c) {
				t.Fatalf("CastSpell failed: %v", g.TakePendingError())
			}
			if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
				t.Fatalf("ResolveStack: %v", err)
			}
			if got := g.Player(p).Life - before; got != 3+tc.want {
				t.Errorf("life gained = %d, want %d", got, 3+tc.want)
			}
		})
	}
}
