package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
	"github.com/jczastkiewicz/crucible/internal/mana"
)

// plotWith gives p mana of color, queues generic payments in that color and
// plots card.
func plotWith(g *engine.Game, p engine.PlayerID, card engine.CardID, c *engine.ScriptedController, color mana.Colors, shard mana.Shard, n int) bool {
	g.Player(p).ManaPool.Add(color, n)
	for range n {
		c.QueuePayGeneric(shard)
	}
	return g.Plot(p, card, c)
}

// CR 702.170: plotting is a sorcery-speed special action that exiles the card
// from hand for its plot cost; the plotted card is cast from exile without
// paying its mana cost, as a sorcery, on a later turn only.
func TestPlotExilesAndCastsFreeOnALaterTurn(t *testing.T) {
	t.Parallel()
	g, p, other := newTwoPlayerGameOn(t, scenarioDB(t))
	c := engine.NewScriptedController()
	djinn := g.NewCard(corpusCard(t, "Djinn of Fool's Fall"), p, engine.Hand)

	g.SetTurnState(1, other, engine.Main1)
	if plotWith(g, p, djinn, c, mana.Blue, mana.ShardU, 4) {
		t.Fatal("plotted on the opponent's turn")
	}
	g.Player(p).ManaPool = engine.Pool{}

	g.SetTurnState(2, p, engine.Main1)
	if !plotWith(g, p, djinn, c, mana.Blue, mana.ShardU, 4) {
		t.Fatalf("plot failed: %v", g.TakePendingError())
	}
	if d := g.Card(djinn); d.Zone != engine.Exile || !d.Plotted {
		t.Fatalf("after plotting: zone %v plotted %v, want plotted in exile", d.Zone, d.Plotted)
	}
	if got := g.Player(p).ManaPool.Total(); got != 0 {
		t.Errorf("pool %d after paying {3}{U}, want 0", got)
	}
	if g.CastSpell(p, djinn, c) {
		t.Fatal("cast the plotted card the turn it was plotted")
	}

	g.SetTurnState(4, p, engine.Main1)
	if !g.CastSpell(p, djinn, c) {
		t.Fatalf("cast from exile failed: %v", g.TakePendingError())
	}
	if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
		t.Fatal(err)
	}
	if d := g.Card(djinn); d.Zone != engine.Battlefield || d.Plotted {
		t.Errorf("after the cast: zone %v plotted %v, want on the battlefield", d.Zone, d.Plotted)
	}
}

// Doc Aurlock, Grizzled Genius: plotting from hand costs {2} less
// (ReduceCost | ValidSpell$ Static.Plotting).
func TestPlottingCostReduction(t *testing.T) {
	t.Parallel()
	g, p, _ := newTwoPlayerGameOn(t, scenarioDB(t))
	g.SetTurnState(1, p, engine.Main1)
	g.NewCard(corpusCard(t, "Doc Aurlock, Grizzled Genius"), p, engine.Battlefield)
	djinn := g.NewCard(corpusCard(t, "Djinn of Fool's Fall"), p, engine.Hand)
	c := engine.NewScriptedController()
	if !plotWith(g, p, djinn, c, mana.Blue, mana.ShardU, 2) {
		t.Fatalf("plot for {1}{U} failed: %v", g.TakePendingError())
	}
}

// Mode$ PlotZone (Fblthp, Lost on the Range): the top card of its
// controller's library has plot for its mana cost and may be plotted from
// there.
func TestPlotZoneFromTheTopOfTheLibrary(t *testing.T) {
	t.Parallel()
	g, p, _ := newTwoPlayerGameOn(t, scenarioDB(t))
	g.SetTurnState(1, p, engine.Main1)
	bears := g.NewCard(corpusCard(t, "Grizzly Bears"), p, engine.Library)
	libraryCards(t, g, p, 2)
	g.NewCard(corpusCard(t, "Fblthp, Lost on the Range"), p, engine.Battlefield)
	c := engine.NewScriptedController()
	// Fblthp's grant of Plot to the top card applies once continuous effects
	// are rebuilt, at the state-based-action check.
	engine.CheckStateBasedActions(g, c)
	if !plotWith(g, p, bears, c, mana.Green, mana.ShardG, 2) {
		t.Fatalf("plot from the library failed: %v", g.TakePendingError())
	}
	if d := g.Card(bears); d.Zone != engine.Exile || !d.Plotted {
		t.Errorf("zone %v plotted %v, want plotted in exile", d.Zone, d.Plotted)
	}
}
