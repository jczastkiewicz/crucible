package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
	"github.com/jczastkiewicz/crucible/internal/mana"
)

// AlternateAdditionalCost (GameActionUtil.getAdditionalCostSpell): Spark
// Harvest costs {B} plus sacrificing a creature or paying {3}{B}; the caster
// picks, and pays only the alternative picked.
func TestAlternateAdditionalCostPaysTheChosenAlternative(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name         string
		pick         int
		pool         int
		wantFodder   engine.ZoneType
		wantPoolLeft int
	}{
		{"sacrifice", 0, 1, engine.Graveyard, 0},
		{"mana", 1, 5, engine.Battlefield, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			g, p, other := newTwoPlayerGameOn(t, scenarioDB(t))
			g.SetTurnState(1, p, engine.Main1)
			fodder := g.NewCard(corpusCard(t, "Grizzly Bears"), p, engine.Battlefield)
			victim := g.NewCard(corpusCard(t, "Grizzly Bears"), other, engine.Battlefield)
			harvest := g.NewCard(corpusCard(t, "Spark Harvest"), p, engine.Hand)
			g.Player(p).ManaPool.Add(mana.Black, tc.pool)
			c := engine.NewScriptedController()
			c.QueueOption(tc.pick)
			c.QueueTargets([]engine.EntityID{engine.CardEntity(victim)})
			if tc.pick == 1 {
				for range 3 {
					c.QueuePayGeneric(mana.ShardB)
				}
			}
			c.QueueSacrificeChoice([]engine.CardID{fodder})

			if !g.CastSpell(p, harvest, c) {
				t.Fatalf("cast failed: %v", g.TakePendingError())
			}
			if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
				t.Fatalf("resolve: %v", err)
			}
			if got := g.Card(victim).Zone; got != engine.Graveyard {
				t.Errorf("target in %v, want Graveyard", got)
			}
			if got := g.Card(fodder).Zone; got != tc.wantFodder {
				t.Errorf("own Grizzly Bears in %v, want %v", got, tc.wantFodder)
			}
			if got := g.Player(p).ManaPool.Total(); got != tc.wantPoolLeft {
				t.Errorf("%d mana left, want %d", got, tc.wantPoolLeft)
			}
		})
	}
}

// With nothing to sacrifice only {3}{B} is offered, so {B} alone cannot cast
// it: the additional cost is never skipped.
func TestAlternateAdditionalCostIsNeverSkipped(t *testing.T) {
	t.Parallel()

	g, p, other := newTwoPlayerGameOn(t, scenarioDB(t))
	g.SetTurnState(1, p, engine.Main1)
	victim := g.NewCard(corpusCard(t, "Grizzly Bears"), other, engine.Battlefield)
	harvest := g.NewCard(corpusCard(t, "Spark Harvest"), p, engine.Hand)
	g.Player(p).ManaPool.Add(mana.Black, 1)
	c := engine.NewScriptedController()
	c.QueueTargets([]engine.EntityID{engine.CardEntity(victim)})
	for range 3 {
		c.QueuePayGeneric(mana.ShardB)
	}

	if g.CastSpell(p, harvest, c) {
		t.Fatal("Spark Harvest was cast for {B} without its additional cost")
	}
	if got := g.Card(harvest).Zone; got != engine.Hand {
		t.Errorf("Spark Harvest in %v, want Hand", got)
	}
}
