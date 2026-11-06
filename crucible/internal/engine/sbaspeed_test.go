package engine_test

import (
	"fmt"
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
	"github.com/jczastkiewicz/crucible/internal/mana"
)

// CR 704.5z: a player with no speed who controls a permanent with "Start your
// engines!" has speed 1; one who already has speed keeps it.
func TestStartYourEnginesGivesSpeedOne(t *testing.T) {
	t.Parallel()

	g, p, other := newTwoPlayerGameOn(t, scenarioDB(t))
	g.NewCard(corpusCard(t, "Burnout Bashtronaut"), p, engine.Battlefield)
	g.Player(other).Speed = 3
	g.NewCard(corpusCard(t, "Burnout Bashtronaut"), other, engine.Battlefield)
	sba(g)
	if got := g.Player(p).Speed; got != 1 {
		t.Errorf("speed = %d, want 1", got)
	}
	if got := g.Player(other).Speed; got != 3 {
		t.Errorf("a player who already had speed 3 now has %d", got)
	}
}

// Spikeshell Harrier's speed clause compares TargetedController$Speed
// against the inline operand PlayerCountDefinedNonTargetedController$
// HighestSpeed (SpellAbilityCondition.java:224-227, AbilityUtils.java:385-387,
// :1037-1039): the bounced creature's controller loses 1 speed only when it
// is faster than every other player; in two-player that is the caster.
func TestSpikeshellHarrierSlowsOnlyTheFastestOpponent(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ mine, want int }{{1, 2}, {3, 3}, {4, 3}} {
		t.Run(fmt.Sprintf("caster speed %d", tc.mine), func(t *testing.T) {
			t.Parallel()
			g, p, other := newTwoPlayerGameOn(t, scenarioDB(t))
			g.SetTurnState(1, p, engine.Main1)
			bears := g.NewCard(corpusCard(t, "Grizzly Bears"), other, engine.Battlefield)
			g.Player(other).Speed, g.Player(p).Speed = 3, tc.mine
			harrier := g.NewCard(corpusCard(t, "Spikeshell Harrier"), p, engine.Hand)
			c := engine.NewScriptedController()
			c.QueueTargets([]engine.EntityID{engine.CardEntity(bears)})
			g.Player(p).ManaPool.Add(mana.Blue, 1)
			g.Player(p).ManaPool.Add(mana.White, 4)
			for range 4 {
				c.QueuePayGeneric(mana.ShardW)
			}
			if !g.CastSpell(p, harrier, c) {
				t.Fatalf("CastSpell: %v", g.TakePendingError())
			}
			if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
				t.Fatal(err)
			}
			if z := g.Card(bears).Zone; z != engine.Hand {
				t.Errorf("bounced creature zone = %v, want Hand", z)
			}
			if got := g.Player(other).Speed; got != tc.want {
				t.Errorf("opponent speed = %d, want %d", got, tc.want)
			}
		})
	}
}
