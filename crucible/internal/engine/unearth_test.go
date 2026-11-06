package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
	"github.com/jczastkiewicz/crucible/internal/mana"
)

// Unearth (CR 702.84a, ChangeZoneEffect.java:680-698): Dregscape Zombie's
// Unearth {B} returns it from the graveyard with haste; it is exiled at the
// beginning of the next end step, and exiled instead if it would die first.
func TestUnearthReturnsWithHasteAndEndsInExile(t *testing.T) {
	t.Parallel()

	for _, dies := range []bool{false, true} {
		g, p, _ := newTwoPlayerGameOn(t, scenarioDB(t))
		g.SetTurnState(1, p, engine.Main1)
		zombie := g.NewCard(corpusCard(t, "Dregscape Zombie"), p, engine.Graveyard)
		g.Player(p).ManaPool.Add(mana.Black, 1)
		c := engine.NewScriptedController()
		if !g.ActivateAbility(p, zombie, 0, c) {
			t.Fatal("Unearth {B} could not be activated from the graveyard")
		}
		if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
			t.Fatal(err)
		}
		engine.CheckStateBasedActions(g, c)
		if got := g.Card(zombie).Zone; got != engine.Battlefield {
			t.Fatalf("unearthed Dregscape Zombie in %v, want the battlefield", got)
		}
		if !g.Card(zombie).HasKeyword("Haste") {
			t.Error("unearthed Dregscape Zombie has no haste")
		}
		if dies {
			g.Card(zombie).Damage.Mark(20, false)
			sba(g)
		} else {
			toStep(t, g, c, p, engine.EndOfTurn, nil)
		}
		if got := g.Card(zombie).Zone; got != engine.Exile {
			t.Errorf("dies=%v: Dregscape Zombie in %v, want exile", dies, got)
		}
	}
}
