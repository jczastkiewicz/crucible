package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
	"github.com/jczastkiewicz/crucible/internal/mana"
)

// Aether Tide: "As an additional cost, discard X creature cards", X announced
// with the spell (Count$xPaid); X targets are chosen after it.
func TestAetherTideDiscardsAnnouncedXCreatureCards(t *testing.T) {
	t.Parallel()
	g, p, other := newTwoPlayerGameOn(t, scenarioDB(t))
	c := engine.NewScriptedController()
	g.SetTurnState(1, p, engine.Main1)
	victim := g.NewCard(corpusCard(t, "Grizzly Bears"), other, engine.Battlefield)
	fodder := g.NewCard(corpusCard(t, "Grizzly Bears"), p, engine.Hand)
	spell := g.NewCard(corpusCard(t, "Aether Tide"), p, engine.Hand)
	g.Player(p).ManaPool.Add(mana.Blue, 3)
	c.QueuePayX(1)
	c.QueueTargets([]engine.EntityID{engine.CardEntity(victim)})
	c.QueueDiscardChoice([]engine.CardID{fodder})
	c.QueuePayGeneric(mana.ShardU)
	if !g.CastSpell(p, spell, c) {
		t.Fatalf("CastSpell failed: %v", g.TakePendingError())
	}
	if z := g.Card(fodder).Zone; z != engine.Graveyard {
		t.Errorf("discarded creature is in %v, want Graveyard", z)
	}
	if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
		t.Fatalf("ResolveStack: %v (pending %v)", err, g.TakePendingError())
	}
	if z := g.Card(victim).Zone; z != engine.Hand {
		t.Errorf("target is in %v, want Hand", z)
	}
}

// Crashing Wave and Foggy Swamp Visions: "waterbend X" is the only additional
// cost, so the spell line's printed mana is no second one.
func TestWaterbendXSpellsAreCastWithTheirAnnouncedX(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name  string
		setup func(g *engine.Game, p, other engine.PlayerID, c *engine.ScriptedController)
	}{
		{"Crashing Wave", func(g *engine.Game, p, other engine.PlayerID, c *engine.ScriptedController) {
			victim := g.NewCard(corpusCard(t, "Grizzly Bears"), other, engine.Battlefield)
			c.QueueTargets([]engine.EntityID{engine.CardEntity(victim)})
		}},
		{"Foggy Swamp Visions", func(g *engine.Game, p, other engine.PlayerID, c *engine.ScriptedController) {
			dead := g.NewCard(corpusCard(t, "Grizzly Bears"), other, engine.Graveyard)
			c.QueueTargets([]engine.EntityID{engine.CardEntity(dead)})
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			g, p, other := newTwoPlayerGameOn(t, scenarioDB(t))
			c := engine.NewScriptedController()
			g.SetTurnState(1, p, engine.Main1)
			helper := g.NewCard(corpusCard(t, "Grizzly Bears"), p, engine.Battlefield)
			spell := g.NewCard(corpusCard(t, tc.name), p, engine.Hand)
			g.Player(p).ManaPool.Add(mana.Blue, 2)
			g.Player(p).ManaPool.Add(mana.Black, 3)
			c.QueuePayX(1)
			c.QueueCardChoice([]engine.CardID{helper})
			tc.setup(g, p, other, c)
			c.QueuePayGeneric(mana.ShardU)
			c.QueuePayGeneric(mana.ShardB)
			if !g.CastSpell(p, spell, c) {
				t.Fatalf("CastSpell failed: %v", g.TakePendingError())
			}
			if !g.Card(helper).Tapped {
				t.Errorf("the helper was not tapped for the waterbend")
			}
		})
	}
}
