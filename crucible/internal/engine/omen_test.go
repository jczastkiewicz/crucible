package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
	"github.com/jczastkiewicz/crucible/internal/mana"
)

// An Omen spell is shuffled into its owner's library only as it resolves
// (CR 720.3d; the Omen replacement's Fizzle$ False needs the key only
// MagicStack.finishResolving sets): Signaling Roar resolved goes to the
// library, countered it goes to the graveyard like any other spell.
func TestOmenShufflesOnlyWhenItResolves(t *testing.T) {
	t.Parallel()

	for _, counter := range []bool{false, true} {
		g, p, _ := newTwoPlayerGameOn(t, scenarioDB(t))
		g.SetTurnState(1, p, engine.Main1)
		roar := g.NewCard(corpusCard(t, "Riling Dawnbreaker"), p, engine.Hand)
		g.Player(p).ManaPool.Add(mana.White, 2)
		c := engine.NewScriptedController()
		c.QueuePayGeneric(mana.ShardW)
		if !g.CastFace(p, roar, c) {
			t.Fatalf("Signaling Roar not cast: %v", g.TakePendingError())
		}
		want := engine.Library
		if counter {
			want = engine.Graveyard
			if _, err := resolveNow(t, g, p, c, []engine.EntityID{engine.CardEntity(roar)}, "DB$ Counter | TargetType$ Spell | ValidTgts$ Card"); err != nil {
				t.Fatalf("Counter: %v", err)
			}
		}
		if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
			t.Fatal(err)
		}
		if got := g.Card(roar).Zone; got != want {
			t.Errorf("countered %v: Signaling Roar in %v, want %v", counter, got, want)
		}
	}
}
