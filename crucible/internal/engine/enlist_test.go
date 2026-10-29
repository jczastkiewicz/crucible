package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
)

// Enlist (CR 702.154, CostEnlist.java:76-91): Argivian Cavalier attacks and
// taps a nonattacking Grizzly Bears, getting +2/+0, Bears' power. A creature
// with summoning sickness is not offered.
func TestEnlistTapsACreatureAndPumpsTheAttacker(t *testing.T) {
	t.Parallel()

	g, a, _ := newTwoPlayerGameOn(t, scenarioDB(t))
	g.SetTurnState(1, a, engine.DeclareAttackers)
	cavalier := g.NewCard(corpusCard(t, "Argivian Cavalier"), a, engine.Battlefield)
	bears := g.NewCard(corpusCard(t, "Grizzly Bears"), a, engine.Battlefield)
	sick := g.NewCard(corpusCard(t, "Grizzly Bears"), a, engine.Battlefield)
	g.Card(cavalier).SummonSick = false
	g.Card(bears).SummonSick = false
	g.Card(sick).SummonSick = true
	offered := &offerRecorder{ScriptedController: engine.NewScriptedController()}
	offered.QueueAttackers([]engine.CardID{cavalier})
	offered.QueueCardChoice([]engine.CardID{bears})

	declareAttackers(t, g, offered)
	if err := g.ResolveStack(engine.NewRegistry(), offered); err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if !g.Card(bears).Tapped {
		t.Error("the enlisted Grizzly Bears is untapped")
	}
	if got, _ := g.Card(cavalier).Power(); got != 4 {
		t.Errorf("Argivian Cavalier has power %d, want 4", got)
	}
	if len(offered.offers) != 1 || len(offered.offers[0]) != 1 || offered.offers[0][0] != bears {
		t.Errorf("offered %v, want only the untapped, unsick Grizzly Bears", offered.offers)
	}
}

// Mode$ Enlisted (Guardian of New Benalia): enlisting fires its scry 2
// (TriggerEnlisted.java:26-39).
func TestEnlistedFiresWhenTheAttackerEnlists(t *testing.T) {
	t.Parallel()

	g, a, _ := newTwoPlayerGameOn(t, scenarioDB(t))
	g.SetTurnState(1, a, engine.DeclareAttackers)
	guardian := g.NewCard(corpusCard(t, "Guardian of New Benalia"), a, engine.Battlefield)
	bears := g.NewCard(corpusCard(t, "Grizzly Bears"), a, engine.Battlefield)
	g.Card(guardian).SummonSick = false
	g.Card(bears).SummonSick = false
	lib := libraryCards(t, g, a, 3)
	c := engine.NewScriptedController()
	c.QueueAttackers([]engine.CardID{guardian})
	c.QueueCardChoice([]engine.CardID{bears})
	c.QueueScry(nil, lib[:2])

	declareAttackers(t, g, c)
	if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if top := g.Zone(engine.Library, a).Cards()[0]; top != lib[2] {
		t.Errorf("library top is %v, want %v (scry 2 bottomed the first two)", top, lib[2])
	}
}
