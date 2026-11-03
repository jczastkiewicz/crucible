package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
	"github.com/jczastkiewicz/crucible/internal/mana"
)

// A pumped keyword brings the trigger it stands for (ADR-0038 decision 4,
// Card.addChangedCardKeywords): Cauldron of Souls' KW$ Persist returns a dying
// Grizzly Bears with a -1/-1 counter.
func TestPumpedPersistReturnsTheCreature(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGameOn(t, scenarioDB(t))
	bears := g.NewCard(corpusCard(t, "Grizzly Bears"), p, engine.Battlefield)
	c := engine.NewScriptedController()
	if _, err := resolveNow(t, g, p, c, []engine.EntityID{engine.CardEntity(bears)},
		"DB$ Pump | ValidTgts$ Creature | KW$ Persist"); err != nil {
		t.Fatal(err)
	}
	g.Card(bears).Damage.Mark(20, false)
	sba(g)
	if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
		t.Fatal(err)
	}
	for _, id := range g.Zone(engine.Battlefield, p).Cards() {
		if g.Card(id).Def == g.Card(bears).Def {
			if n := g.Card(id).Counters.Count(engine.M1M1); n != 1 {
				t.Errorf("returned with %d -1/-1 counters, want 1", n)
			}
			return
		}
	}
	t.Fatal("Grizzly Bears did not return through its pumped persist")
}

// A pumped activated keyword's cost names the card's own mana cost:
// Cursecloth Wrappings gives a graveyard Grizzly Bears embalm {1}{G}, and
// activating it exiles the card for a token copy.
func TestPumpedEmbalmIsActivatedForTheCardsManaCost(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGameOn(t, scenarioDB(t))
	bears := g.NewCard(corpusCard(t, "Grizzly Bears"), p, engine.Graveyard)
	if _, err := resolveNow(t, g, p, engine.NewScriptedController(), []engine.EntityID{engine.CardEntity(bears)},
		"DB$ Pump | TgtZone$ Graveyard | ValidTgts$ Creature.YouOwn | PumpZone$ Graveyard | KW$ Embalm:CardManaCost"); err != nil {
		t.Fatal(err)
	}
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).ManaPool.Add(mana.Green, 2)
	c := engine.NewScriptedController()
	c.QueuePayGeneric(mana.ShardG)
	if !g.ActivateAbility(p, bears, 0, c) {
		t.Fatal("the pumped embalm could not be activated for {1}{G}")
	}
	if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
		t.Fatal(err)
	}
	if got := g.Card(bears).Zone; got != engine.Exile {
		t.Errorf("Grizzly Bears is in %v, want exile", got)
	}
	if left := g.Player(p).ManaPool.Total(); left != 0 {
		t.Errorf("%d mana left in the pool, want the embalm cost {1}{G} paid in full", left)
	}
	for _, id := range g.Zone(engine.Battlefield, p).Cards() {
		if g.Card(id).IsToken && g.Card(id).Name() == "Grizzly Bears" {
			return
		}
	}
	t.Error("no token copy of Grizzly Bears entered")
}
