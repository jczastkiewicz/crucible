package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
	"github.com/jczastkiewicz/crucible/internal/mana"
)

// The probe names the colour a colour-blocked card lacks (MET-12): Doom Blade
// with a Swamp and a Forest is castable; with a Forest and a Mountain it
// wants black; Phyrexian Arena ({1}{B}{B}) with one Swamp and two Forests
// wants a second black source although black is there.
func TestProbeNamesTheMissingColour(t *testing.T) {
	t.Parallel()
	g, p, _ := newTwoPlayerGameOn(t, scenarioDB(t))
	g.SetTurnState(1, p, engine.Main1)
	blade := g.NewCard(corpusCard(t, "Doom Blade"), p, engine.Hand)
	g.NewCard(corpusCard(t, "Forest"), p, engine.Battlefield)
	g.NewCard(corpusCard(t, "Mountain"), p, engine.Battlefield)
	if got := probeFor(t, g, p, blade); got.Blocked&engine.BlockColorUnavailable == 0 || got.MissingColors != mana.Black {
		t.Errorf("Doom Blade on Forest and Mountain: %+v, want black missing", got)
	}

	h, q, _ := newTwoPlayerGameOn(t, scenarioDB(t))
	h.SetTurnState(1, q, engine.Main1)
	arena := h.NewCard(corpusCard(t, "Phyrexian Arena"), q, engine.Hand)
	h.NewCard(corpusCard(t, "Swamp"), q, engine.Battlefield)
	h.NewCard(corpusCard(t, "Forest"), q, engine.Battlefield)
	h.NewCard(corpusCard(t, "Forest"), q, engine.Battlefield)
	if got := probeFor(t, h, q, arena); got.MissingColors != mana.Black {
		t.Errorf("Phyrexian Arena on one Swamp: %+v, want black missing", got)
	}
}

// Mana left in a pool when a step ends is lost and reported as ManaLost
// (CR 500.4; MET-13), with the player and the amount.
func TestEmptyingAPoolEmitsTheManaLost(t *testing.T) {
	t.Parallel()
	g, p, _ := newTwoPlayerGameOn(t, scenarioDB(t))
	g.SetTurnState(1, p, engine.Main1)
	sink := &recordingSink{}
	g.SetSink(sink)
	g.Player(p).ManaPool.Add(mana.Green, 2)
	g.AdvancePhase(engine.NewScriptedController())
	var lost []engine.Event
	for _, e := range sink.events {
		if e.Kind == engine.ManaLost {
			lost = append(lost, e)
		}
	}
	if len(lost) != 1 || lost[0].Actor != p || lost[0].Amount != 2 {
		t.Errorf("ManaLost events %+v, want one of 2 for the player", lost)
	}
	if engine.ManaLost.String() != "ManaLost" {
		t.Errorf("name %q", engine.ManaLost.String())
	}
}
