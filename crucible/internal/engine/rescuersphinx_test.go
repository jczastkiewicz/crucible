package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
	"github.com/jczastkiewicz/crucible/internal/mana"
)

// Rescuer Sphinx: "As it enters, you may return a nonland permanent you
// control to its owner's hand. If you do, it enters with a +1/+1 counter."
// The cost is the UnlessCost$ of an UnlessSwitched$ ETB PutCounter, so the
// counter comes only from paying it.
func TestRescuerSphinxEntersWithACounterOnlyWhenAPermanentIsReturned(t *testing.T) {
	t.Parallel()

	for _, pay := range []bool{true, false} {
		g, p, _ := newTwoPlayerGameOn(t, scenarioDB(t))
		c := engine.NewScriptedController()
		g.SetTurnState(1, p, engine.Main1)
		bears := g.NewCard(corpusCard(t, "Grizzly Bears"), p, engine.Battlefield)
		sphinx := g.NewCard(corpusCard(t, "Rescuer Sphinx"), p, engine.Hand)
		g.Player(p).ManaPool.Add(mana.Blue, 4)
		for i := 0; i < 2; i++ {
			c.QueuePayGeneric(mana.ShardU)
		}
		c.QueueConfirmPayCost(pay)
		if pay {
			c.QueueReturnChoice([]engine.CardID{bears})
		}
		if !g.CastSpell(p, sphinx, c) {
			t.Fatalf("pay=%v: CastSpell failed: %v", pay, g.TakePendingError())
		}
		if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
			t.Fatalf("pay=%v: ResolveStack: %v", pay, err)
		}
		wantCounters, wantBears := 0, engine.Battlefield
		if pay {
			wantCounters, wantBears = 1, engine.Hand
		}
		if got := g.Card(sphinx).Counters.Count(engine.P1P1); got != wantCounters {
			t.Errorf("pay=%v: counters = %d, want %d", pay, got, wantCounters)
		}
		if got := g.Card(bears).Zone; got != wantBears {
			t.Errorf("pay=%v: bears in %v, want %v", pay, got, wantBears)
		}
	}
}
