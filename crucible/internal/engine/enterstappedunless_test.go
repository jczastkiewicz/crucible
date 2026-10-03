package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
)

// "As this enters, you may pay 2 life. If you don't, it enters tapped"
// (Temple Garden, 27 real lines): AbilityUtils.handleUnlessCost on the
// ETBTapped replacement. Paying keeps the land untapped and costs 2 life;
// declining leaves it tapped.
func TestShockLandEntersTappedUnlessItsControllerPaysLife(t *testing.T) {
	t.Parallel()

	for _, pay := range []bool{true, false} {
		g, p, _ := newTwoPlayerGameOn(t, scenarioDB(t))
		g.SetTurnState(1, p, engine.Main1)
		land := g.NewCard(corpusCard(t, "Temple Garden"), p, engine.Hand)
		sba(g)
		c := engine.NewScriptedController()
		c.QueueConfirmPayCost(pay)
		if !g.PlayLand(p, land, c) {
			t.Fatalf("pay %v: PlayLand failed", pay)
		}
		if err := g.TakePendingError(); err != nil {
			t.Fatalf("pay %v: %v", pay, err)
		}
		wantLife := 20
		if pay {
			wantLife = 18
		}
		if got := g.Card(land).Tapped; got == pay {
			t.Errorf("pay %v: Temple Garden tapped = %v, want %v", pay, got, !pay)
		}
		if got := g.Player(p).Life; got != wantLife {
			t.Errorf("pay %v: life = %d, want %d", pay, got, wantLife)
		}
	}
}

// A shock land cannot pay with too little life (CR 119.4): it enters tapped
// whatever the controller would answer, and nobody is asked.
func TestShockLandWithTooLittleLifeEntersTapped(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGameOn(t, scenarioDB(t))
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life = 1
	land := g.NewCard(corpusCard(t, "Temple Garden"), p, engine.Hand)
	sba(g)
	c := engine.NewScriptedController()
	c.QueueConfirmPayCost(true)
	if !g.PlayLand(p, land, c) {
		t.Fatal("PlayLand failed")
	}
	if !g.Card(land).Tapped || g.Player(p).Life != 1 {
		t.Errorf("tapped = %v, life = %d; want tapped at 1 life", g.Card(land).Tapped, g.Player(p).Life)
	}
}

// CR 614.12 with CR 305.7: Blood Moon makes the entering Temple Garden a
// Mountain without the shock replacement, so nobody is asked and no life is
// paid; it enters untapped.
func TestBloodMoonRemovesAShockLandsReplacement(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGameOn(t, scenarioDB(t))
	g.SetTurnState(1, p, engine.Main1)
	g.NewCard(corpusCard(t, "Blood Moon"), p, engine.Battlefield)
	land := g.NewCard(corpusCard(t, "Temple Garden"), p, engine.Hand)
	sba(g)
	c := engine.NewScriptedController()
	c.QueueConfirmPayCost(true)
	if !g.PlayLand(p, land, c) {
		t.Fatal("PlayLand failed")
	}
	if g.Card(land).Tapped || g.Player(p).Life != 20 {
		t.Errorf("tapped = %v, life = %d; want untapped at 20 life", g.Card(land).Tapped, g.Player(p).Life)
	}
}

// "Unless you reveal a Plains or Island card from your hand" (Port Town,
// 19 real lines): the reveal is the cost. With a matching card in hand the
// controller may pay and keep the land untapped; with none it enters tapped.
func TestRevealLandEntersTappedUnlessItsControllerRevealsAMatchingCard(t *testing.T) {
	t.Parallel()

	for _, hasIsland := range []bool{true, false} {
		g, p, _ := newTwoPlayerGameOn(t, scenarioDB(t))
		g.SetTurnState(1, p, engine.Main1)
		land := g.NewCard(corpusCard(t, "Port Town"), p, engine.Hand)
		if hasIsland {
			g.NewCard(corpusCard(t, "Island"), p, engine.Hand)
		}
		sba(g)
		c := engine.NewScriptedController()
		c.QueueConfirmPayCost(true)
		if !g.PlayLand(p, land, c) {
			t.Fatalf("island in hand %v: PlayLand failed", hasIsland)
		}
		if err := g.TakePendingError(); err != nil {
			t.Fatalf("island in hand %v: %v", hasIsland, err)
		}
		if got := g.Card(land).Tapped; got == hasIsland {
			t.Errorf("island in hand %v: Port Town tapped = %v, want %v", hasIsland, got, !hasIsland)
		}
	}
}
