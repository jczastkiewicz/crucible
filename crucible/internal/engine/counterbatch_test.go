package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
)

const putCounterLine = "DB$ PutCounter | ValidTgts$ Creature | CounterType$ P1P1 | CounterNum$ 1"

func fillLibrary(t *testing.T, g *engine.Game, p engine.PlayerID, n int) {
	t.Helper()
	for range n {
		g.NewCard(corpusCard(t, "Forest"), p, engine.Library)
	}
}

func handSize(g *engine.Game, p engine.PlayerID) int { return len(g.Zone(engine.Hand, p).Cards()) }

// Mode$ CounterAddedAll | CounterType$ P1P1 | Valid$ Human.YouCtrl | ActivationLimit$ 1
// (Cloaked Cadet): +1/+1 counters on a Human its controller has draw a card,
// once per turn; a creature that is no Human does not.
func TestCounterAddedAllFiresForMatchingCardsOncePerTurn(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGameOn(t, scenarioDB(t))
	g.SetTurnState(1, p, engine.Main1)
	cadet := g.NewCard(corpusCard(t, "Cloaked Cadet"), p, engine.Battlefield)
	bear := g.NewCard(corpusCard(t, "Grizzly Bears"), p, engine.Battlefield)
	fillLibrary(t, g, p, 3)
	sba(g)
	c := engine.NewScriptedController()
	for _, tc := range []struct {
		target engine.CardID
		want   int
	}{{bear, 0}, {cadet, 1}, {cadet, 1}} {
		if err := resolveTargeting(t, g, p, c, []engine.EntityID{engine.CardEntity(tc.target)}, putCounterLine); err != nil {
			t.Fatalf("resolve: %v", err)
		}
		if got := handSize(g, p); got != tc.want {
			t.Fatalf("hand = %d, want %d", got, tc.want)
		}
	}
}

// Mode$ CounterPlayerAddedAll | ValidObject$ Creature.YouDontCtrl+inRealZoneBattlefield
// | ValidSource$ You (Generous Patron): counters its controller puts on an
// opposing creature draw a card; counters on its own creature do not.
func TestCounterPlayerAddedAllReadsWhoPlacedOnWhat(t *testing.T) {
	t.Parallel()

	g, p, q := newTwoPlayerGameOn(t, scenarioDB(t))
	g.SetTurnState(1, p, engine.Main1)
	g.NewCard(corpusCard(t, "Generous Patron"), p, engine.Battlefield)
	own := g.NewCard(corpusCard(t, "Grizzly Bears"), p, engine.Battlefield)
	theirs := g.NewCard(corpusCard(t, "Grizzly Bears"), q, engine.Battlefield)
	fillLibrary(t, g, p, 2)
	sba(g)
	c := engine.NewScriptedController()
	for _, tc := range []struct {
		target engine.CardID
		want   int
	}{{own, 0}, {theirs, 1}} {
		if err := resolveTargeting(t, g, p, c, []engine.EntityID{engine.CardEntity(tc.target)}, putCounterLine); err != nil {
			t.Fatalf("resolve: %v", err)
		}
		if got := handSize(g, p); got != tc.want {
			t.Fatalf("hand = %d, want %d", got, tc.want)
		}
	}
}

// Mode$ CounterTypeAddedAll | FirstTime$ True (Danny Pink's granted trigger):
// only the first counters a creature gets in a turn draw.
func TestCounterTypeAddedAllFiresOnlyForTheFirstPlacementOfTheTurn(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGameOn(t, scenarioDB(t))
	g.SetTurnState(1, p, engine.Main1)
	g.NewCard(corpusCard(t, "Danny Pink"), p, engine.Battlefield)
	bear := g.NewCard(corpusCard(t, "Grizzly Bears"), p, engine.Battlefield)
	fillLibrary(t, g, p, 3)
	sba(g)
	c := engine.NewScriptedController()
	for range 2 {
		if err := resolveTargeting(t, g, p, c, []engine.EntityID{engine.CardEntity(bear)}, putCounterLine); err != nil {
			t.Fatalf("resolve: %v", err)
		}
	}
	if got := handSize(g, p); got != 1 {
		t.Errorf("hand = %d after two placements, want 1", got)
	}
}
