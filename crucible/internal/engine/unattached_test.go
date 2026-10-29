package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
)

// Mode$ Unattached | ValidAttachment$ Card.Self | ValidObject$ Permanent
// (Grafted Wargear): moving it to another creature sacrifices the creature
// it left, and so does the Equipment leaving the battlefield
// (Card.unattachFromEntity, Card.java:4013-4019).
func TestUnattachedSacrificesTheCreatureLeftBehind(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGameOn(t, scenarioDB(t))
	g.SetTurnState(1, p, engine.Main1)
	wargear := g.NewCard(corpusCard(t, "Grafted Wargear"), p, engine.Battlefield)
	first := g.NewCard(corpusCard(t, "Grizzly Bears"), p, engine.Battlefield)
	second := g.NewCard(corpusCard(t, "Grizzly Bears"), p, engine.Battlefield)
	c := engine.NewScriptedController()
	g.Attach(wargear, first)
	g.Attach(wargear, second)
	engine.CheckStateBasedActions(g, c)
	if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
		t.Fatalf("resolve after moving: %v", err)
	}
	if got := g.Card(first).Zone; got != engine.Graveyard {
		t.Fatalf("the creature Grafted Wargear left is in %v, want Graveyard", got)
	}

	if err := resolveTargeting(t, g, p, c, []engine.EntityID{engine.CardEntity(wargear)}, "DB$ Destroy | ValidTgts$ Artifact"); err != nil {
		t.Fatalf("destroy: %v", err)
	}
	if got := g.Card(second).Zone; got != engine.Graveyard {
		t.Errorf("the creature equipped when Grafted Wargear died is in %v, want Graveyard", got)
	}
}
