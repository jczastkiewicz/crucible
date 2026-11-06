package engine_test

import (
	"slices"
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
)

func probeFor(t *testing.T, g *engine.Game, p engine.PlayerID, id engine.CardID) engine.CastProbe {
	t.Helper()
	for _, pr := range g.CastabilityProbe(p) {
		if pr.Card == id {
			return pr
		}
	}
	t.Fatalf("card %v not probed", id)
	return engine.CastProbe{}
}

func TestProbeSeparatesColourFromQuantity(t *testing.T) {
	t.Parallel()
	g, p, _ := newTwoPlayerGameOn(t, scenarioDB(t))
	g.SetTurnState(1, p, engine.Main1)
	bolt := g.NewCard(corpusCard(t, "Lightning Bolt"), p, engine.Hand)
	if got := probeFor(t, g, p, bolt); got.Castable || got.Blocked&engine.BlockInsufficientMana == 0 {
		t.Errorf("Bolt with no lands: %+v, want blocked on quantity", got)
	}
	for i := 0; i < 2; i++ {
		g.NewCard(corpusCard(t, "Island"), p, engine.Battlefield)
	}
	got := probeFor(t, g, p, bolt)
	if got.Castable || got.Blocked&engine.BlockColorUnavailable == 0 || got.Blocked&engine.BlockInsufficientMana != 0 {
		t.Errorf("Bolt with two Islands: %+v, want blocked on colour only", got)
	}
	if names := got.Blocked.Names(); !slices.Equal(names, []string{"ColorUnavailable"}) {
		t.Errorf("names %v", names)
	}
	g.NewCard(corpusCard(t, "Mountain"), p, engine.Battlefield)
	if got := probeFor(t, g, p, bolt); !got.Castable || got.Blocked != 0 {
		t.Errorf("Bolt with a Mountain: %+v, want castable", got)
	}
}

func TestProbeReadsTargetsTimingAndLandDrop(t *testing.T) {
	t.Parallel()
	g, p, o := newTwoPlayerGameOn(t, scenarioDB(t))
	g.SetTurnState(1, p, engine.Main1)
	for i := 0; i < 2; i++ {
		g.NewCard(corpusCard(t, "Swamp"), p, engine.Battlefield)
	}
	doom := g.NewCard(corpusCard(t, "Doom Blade"), p, engine.Hand)
	if got := probeFor(t, g, p, doom); got.Blocked&engine.BlockNoLegalTarget == 0 {
		t.Errorf("Doom Blade with no creature: %+v, want no legal target", got)
	}
	g.NewCard(corpusCard(t, "Grizzly Bears"), o, engine.Battlefield)
	if got := probeFor(t, g, p, doom); !got.Castable {
		t.Errorf("Doom Blade with a target: %+v, want castable", got)
	}
	bears := g.NewCard(corpusCard(t, "Grizzly Bears"), p, engine.Hand)
	g.SetTurnState(1, o, engine.Main1)
	if got := probeFor(t, g, p, bears); got.Blocked&engine.BlockTimingRestriction == 0 {
		t.Errorf("a creature on the opponent's turn: %+v, want a timing block", got)
	}
	if got := probeFor(t, g, p, doom); got.Blocked&engine.BlockTimingRestriction != 0 {
		t.Errorf("an instant on the opponent's turn: %+v, want no timing block", got)
	}
	g.SetTurnState(1, p, engine.Main1)
	land := g.NewCard(corpusCard(t, "Forest"), p, engine.Hand)
	if got := probeFor(t, g, p, land); !got.Castable {
		t.Errorf("a land with the drop available: %+v", got)
	}
	g.PlayLand(p, land, engine.NewScriptedController())
	second := g.NewCard(corpusCard(t, "Forest"), p, engine.Hand)
	if got := probeFor(t, g, p, second); got.Castable || got.Blocked&engine.BlockLandDropUsed == 0 {
		t.Errorf("a land after the drop: %+v, want the land drop used", got)
	}
}
