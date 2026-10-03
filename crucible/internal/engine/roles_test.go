package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
	"github.com/jczastkiewicz/crucible/internal/mana"
)

// castMonstrousRage casts a Monstrous Rage on target (a Pump with a Monster
// Role token attached to it) and resolves it.
func castMonstrousRage(t *testing.T, g *engine.Game, p engine.PlayerID, target engine.CardID) {
	t.Helper()
	rage := g.NewCard(corpusCard(t, "Monstrous Rage"), p, engine.Hand)
	g.Player(p).ManaPool.Add(mana.Red, 1)
	c := engine.NewScriptedController()
	c.QueueTargets([]engine.EntityID{engine.CardEntity(target)})
	castThenResolve(t, g, p, rage, c)
}

// Token AttachedTo$ (CR 303.4i): the Role token is created attached to its
// target; and CR 704.5y -- a creature keeps only the newest Role its
// controller put on it, the older one going to the graveyard.
func TestANewRoleReplacesTheOlderOneOfTheSameController(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGameOn(t, scenarioDB(t))
	g.SetTurnState(1, p, engine.Main1)
	bears := g.NewCard(corpusCard(t, "Grizzly Bears"), p, engine.Battlefield)

	castMonstrousRage(t, g, p, bears)
	first := append([]engine.CardID(nil), g.Card(bears).Attachments()...)
	if len(first) != 1 {
		t.Fatalf("after one Monstrous Rage the Bears carry %d attachments, want the Monster Role", len(first))
	}
	castMonstrousRage(t, g, p, bears)
	second := append([]engine.CardID(nil), g.Card(bears).Attachments()...)
	if len(second) != 1 {
		t.Fatalf("after two the Bears carry %d attachments, want 1", len(second))
	}
	if second[0] == first[0] {
		t.Error("the older Role stayed")
	}
	if g.Card(first[0]).Zone == engine.Battlefield {
		t.Error("the older Role is still on the battlefield")
	}
	// +1/+1 from the one Role, +2/+0 twice from the pumps.
	if pw, _ := g.Card(bears).Power(); pw != 2+1+2+2 {
		t.Errorf("Bears power = %d, want 7", pw)
	}
	if tg, _ := g.Card(bears).Toughness(); tg != 2+1 {
		t.Errorf("Bears toughness = %d, want 3: a Role's +1/+1 counted once", tg)
	}
}
