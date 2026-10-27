package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
	"github.com/jczastkiewicz/crucible/internal/mana"
)

// Elektra, Femme Fatale: "you may have her deal 2 damage to you. When you do,
// she deals 4 damage to target creature." Declining deals nothing and does not
// start the reflexive trigger (DamageDealEffect.java:180-185 OptionalDecider$).
func TestElektraDecliningTheSelfDamageDealsNothing(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGameOn(t, scenarioDB(t))
	victim := g.NewCard(creatureDefPT(t, "2", "5"), p, engine.Battlefield)
	elektra := g.NewCard(corpusCard(t, "Elektra, Femme Fatale"), p, engine.Hand)
	g.Player(p).ManaPool.Add(mana.Red, 4)
	c := engine.NewScriptedController()
	for range 4 {
		c.QueuePayGeneric(mana.ShardR)
	}
	c.QueueConfirmEffect(false)
	before := g.Player(p).Life
	if err := castEntrant(t, g, p, elektra, c); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if got := g.Player(p).Life; got != before {
		t.Errorf("life = %d, want %d: the self-damage was declined", got, before)
	}
	if d := g.Card(victim).Damage; d.Marked != 0 {
		t.Errorf("creature damage = %d, want 0: no reflexive trigger without the self-damage", d.Marked)
	}
}
