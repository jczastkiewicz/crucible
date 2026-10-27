package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
	"github.com/jczastkiewicz/crucible/internal/mana"
)

// Tezzeret's Reckoning: three random cards leave the library face down
// (ExileFaceDown$, ChangeZoneEffect.java:1450 -- the card used FaceDown$, which
// the hidden branch never reads).
func TestTezzeretsReckoningExilesThreeCardsFaceDown(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGameOn(t, scenarioDB(t))
	for range 6 {
		g.NewCard(creatureDefPT(t, "1", "1"), p, engine.Library)
	}
	spell := g.NewCard(corpusCard(t, "Tezzeret's Reckoning"), p, engine.Hand)
	g.Player(p).ManaPool.Add(mana.Blue, 2)
	c := engine.NewScriptedController()
	c.QueuePayGeneric(mana.ShardU)
	c.QueuePayGeneric(mana.ShardU)
	if err := castEntrant(t, g, p, spell, c); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	exiled := g.Zone(engine.Exile, p).Cards()
	if len(exiled) != 3 {
		t.Fatalf("exiled %d cards, want 3", len(exiled))
	}
	for _, id := range exiled {
		if !g.Card(id).IsFaceDown() {
			t.Errorf("card %v was exiled face up", id)
		}
	}
}
