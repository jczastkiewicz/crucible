package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
)

// A copy fires the copy modes (MagicStack.java:430-439) and a cast fires only
// SpellCastOrCopy: the copied Gain instant gains 3 twice, and each watcher
// adds 1 per event it sees.
func TestCopyingASpellFiresTheCopyTriggers(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, trigger string
		want          int
	}{
		{"SpellCopy fires for the copy only", "Mode$ SpellCopy | ValidCard$ Instant | ValidActivatingPlayer$ You", 1},
		{"SpellAbilityCopy fires for the copy only", "Mode$ SpellAbilityCopy | ValidCard$ Instant | ValidActivatingPlayer$ You", 1},
		{"SpellCastOrCopy fires for the cast and the copy", "Mode$ SpellCastOrCopy | ValidCard$ Instant | ValidActivatingPlayer$ You", 2},
		{"a copy's controller is the Activator", "Mode$ SpellCopy | ValidCard$ Instant | ValidActivatingPlayer$ Opponent", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			g, p, _ := newTwoPlayerGame(t)
			g.NewCard(triggerWatcherDef(t, "Watcher", tc.trigger+" | Execute$ TrigGain", "TrigGain", "DB$ GainLife | Defined$ You | LifeAmount$ 1"), p, engine.Battlefield)
			spell := g.NewCard(gainInstant(t, "Gain", "W", "3"), p, engine.Hand)
			before := g.Player(p).Life
			if err := castWithCopyWatcher(t, g, p, spell, engine.NewScriptedController(), "Instant", "DB$ CopySpellAbility | Defined$ TriggeredSpellAbility"); err != nil {
				t.Fatal(err)
			}
			if got := g.Player(p).Life - before; got != 6+tc.want {
				t.Errorf("life gained = %d, want %d", got, 6+tc.want)
			}
		})
	}
}
