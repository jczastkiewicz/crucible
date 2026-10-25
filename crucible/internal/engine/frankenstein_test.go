package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
	"github.com/jczastkiewicz/crucible/internal/mana"
)

// Frankenstein's Monster: as it enters, exile X creature cards from your
// graveyard; if you can't, it goes to the graveyard instead. The bare
// ReplaceEffect at the end of the chain is what lets the entry go on.
func TestFrankensteinsMonsterNeedsXCreatureCardsToEnter(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name  string
		cards int
		want  engine.ZoneType
	}{
		{"enough cards", 1, engine.Battlefield},
		{"no creature card", 0, engine.Graveyard},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			g, p, _ := newTwoPlayerGameOn(t, scenarioDB(t))
			g.SetTurnState(1, p, engine.Main1)
			c := engine.NewScriptedController()
			var yard []engine.CardID
			for range tc.cards {
				yard = append(yard, g.NewCard(corpusCard(t, "Grizzly Bears"), p, engine.Graveyard))
			}
			monster := g.NewCard(corpusCard(t, "Frankenstein's Monster"), p, engine.Hand)
			g.Player(p).ManaPool.Add(mana.Black, 3)
			c.QueuePayX(1)
			c.QueuePayGeneric(mana.ShardB)
			if len(yard) > 0 {
				c.QueueCardChoice(yard)
			}
			c.QueueNumberChoice(1)
			c.QueueNumberChoice(0)
			if !g.CastSpell(p, monster, c) {
				t.Fatalf("CastSpell failed: %v", g.TakePendingError())
			}
			if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
				t.Fatalf("ResolveStack: %v (pending %v)", err, g.TakePendingError())
			}
			if z := g.Card(monster).Zone; z != tc.want {
				t.Errorf("the Monster is in %v, want %v", z, tc.want)
			}
			if tc.want == engine.Battlefield {
				if got := g.Card(monster).Counters.Count("P2P0"); got != 1 {
					t.Errorf("+2/+0 counters = %d, want the one share X=1 gives", got)
				}
				if pw, _ := g.Card(monster).Power(); pw != 2 {
					t.Errorf("power = %d, want 2 from the +2/+0 counter", pw)
				}
			}
		})
	}
}
