package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
	"github.com/jczastkiewicz/crucible/pkg/javarand"
)

// Ramses, Assassin Lord: "Whenever a player loses the game, if they were
// attacked this turn by an Assassin you controlled, you win the game."
// wasAttackedThisTurnBy reads the attackers as declared against that player
// (PlayerProperty.java:443-450): an Assassin sent at another player, or a
// non-Assassin sent at the loser, wins nothing.
func TestWasAttackedThisTurnByReadsTheAttackersOfThatPlayer(t *testing.T) {
	t.Parallel()

	db := scenarioDB(t)
	for _, tc := range []struct {
		name     string
		attacker string
		atLoser  bool
		want     bool
	}{
		{"an Assassin attacked the loser", "Royal Assassin", true, true},
		{"an Assassin attacked another player", "Royal Assassin", false, false},
		{"a non-Assassin attacked the loser", "Grizzly Bears", true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			g := engine.NewGame(db, javarand.New(1), []string{"a", "b", "c"})
			ps := g.Players()
			p, loser, other := ps[0], ps[1], ps[2]
			g.SetTurnState(1, p, engine.Main1)
			g.NewCard(corpusCard(t, "Ramses, Assassin Lord"), p, engine.Battlefield)
			attacker := g.NewCard(corpusCard(t, tc.attacker), p, engine.Battlefield)
			c := engine.NewScriptedController()
			c.QueueAttackers([]engine.CardID{attacker})
			target := other
			if tc.atLoser {
				target = loser
			}
			c.QueueAttackTarget(engine.PlayerEntity(target))
			declareAttackers(t, g, c)

			g.Player(loser).Life = 0
			sba(g)
			if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
				t.Fatalf("resolve: %v", err)
			}
			if !g.Player(loser).Lost {
				t.Fatal("the player at 0 life did not lose")
			}
			if got := g.Player(p).Won; got != tc.want {
				t.Errorf("Ramses' controller won = %v, want %v", got, tc.want)
			}
		})
	}
}
