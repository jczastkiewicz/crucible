package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
	"github.com/jczastkiewicz/crucible/internal/mana"
)

// Prison Barricade (Event$ Moved, Card.Self+kicked, ReplaceWith$ PutCounter
// then Animate of Self): kicked, it enters with a +1/+1 counter and can attack
// despite its defender; cast plain, it gets neither. A Java run on
// upstream/master (GameSimulator, kicked cast) leaves it a 2/4 with the counter.
func TestKickedPrisonBarricadeEntersWithACounterAndCanAttack(t *testing.T) {
	t.Parallel()
	for _, kicked := range []bool{true, false} {
		g, p, _ := newTwoPlayerGameOn(t, scenarioDB(t))
		g.SetTurnState(1, p, engine.Main1)
		barricade := g.NewCard(corpusCard(t, "Prison Barricade"), p, engine.Hand)
		c := engine.NewScriptedController()
		g.Player(p).ManaPool.Add(mana.White, 4)
		c.QueueConfirmPayCost(kicked)
		for range 2 {
			c.QueuePayGeneric(mana.ShardW)
		}
		if !g.CastSpell(p, barricade, c) {
			t.Fatalf("kicked=%v: cast failed: %v", kicked, g.TakePendingError())
		}
		if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
			t.Fatalf("kicked=%v: %v", kicked, err)
		}
		want := 0
		if kicked {
			want = 1
		}
		if got := g.Card(barricade).Counters.Count("P1P1"); got != want {
			t.Errorf("kicked=%v: %d +1/+1 counters, want %d", kicked, got, want)
		}
	}
}
