package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
)

// Oath of Druids' target is "Player.OpponentOf TriggeredPlayer+
// withMoreCreaturesThanTriggeredPlayer": the "Than" player is comparedPlayer's
// Defined read with the trigger's refs (PlayerProperty.java:310-316, 511-516),
// the player whose upkeep it is, not the Oath's controller. On the opponent's
// upkeep the Oath's controller, with more creatures than that opponent, is a
// legal target, so the opponent digs up a creature.
func TestOathComparesAgainstTheTriggeredPlayer(t *testing.T) {
	t.Parallel()

	g, p, other := newTwoPlayerGameOn(t, scenarioDB(t))
	g.NewCard(corpusCard(t, "Oath of Druids"), p, engine.Battlefield)
	g.NewCard(corpusCard(t, "Grizzly Bears"), p, engine.Battlefield)
	g.NewCard(corpusCard(t, "Grizzly Bears"), p, engine.Battlefield)
	libraryCards(t, g, p, 5)
	for range 3 {
		g.NewCard(corpusCard(t, "Hill Giant"), other, engine.Library)
	}
	c := engine.NewScriptedController()
	c.QueueTargets([]engine.EntityID{engine.PlayerEntity(p)})
	c.QueueConfirmEffect(true)
	toStep(t, g, c, other, engine.Upkeep, nil)

	n := 0
	for _, id := range g.Zone(engine.Battlefield, other).Cards() {
		if g.Card(id).Def.Name == "Hill Giant" {
			n++
		}
	}
	if n != 1 {
		t.Errorf("Hill Giants on the opponent's battlefield = %d, want 1", n)
	}
}
