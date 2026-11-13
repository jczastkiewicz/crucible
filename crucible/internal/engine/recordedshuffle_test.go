package engine_test

import (
	"slices"
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
)

// orderShuffler answers every library shuffle with order.
type orderShuffler struct{ order []engine.CardID }

func (s orderShuffler) Order(kind engine.ZoneType, _ engine.PlayerID, _ []engine.CardID) ([]engine.CardID, bool) {
	return s.order, kind == engine.Library
}

// A shuffler set on the game chooses a shuffle's order, top first (a
// replay's recorded shuffle, ADR-0059); an answer that is not a permutation
// of the zone leaves the shuffle to the game's random stream.
func TestShufflerChoosesTheShuffledOrder(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGame(t)
	cards := libraryCards(t, g, p, 4)
	want := []engine.CardID{cards[2], cards[0], cards[3], cards[1]}
	g.SetShuffler(orderShuffler{want})
	g.Shuffle(engine.Library, p)
	if got := g.Zone(engine.Library, p).Cards(); !slices.Equal(got, want) {
		t.Errorf("library %v, want the shuffler's %v", got, want)
	}

	for _, bad := range [][]engine.CardID{want[:3], {cards[0], cards[0], cards[1], cards[2]}} {
		g.SetShuffler(orderShuffler{bad})
		g.Shuffle(engine.Library, p)
		got := slices.Clone(g.Zone(engine.Library, p).Cards())
		slices.Sort(got)
		if !slices.Equal(got, cards) {
			t.Errorf("shuffler answer %v: library %v, want the same four cards", bad, got)
		}
	}
}

// AbilityAt numbers a card's activated abilities as ActionActivate takes
// them, and AbilityCount is how many it answers for: what a replay walks to
// find an ability Java named (internal/replay).
func TestAbilityAtNumbersTheActionIndices(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGame(t)
	id := g.NewCard(creatureDefWithAbility(t, "Pinger", "AB$ GainLife | Cost$ T | Defined$ You | LifeAmount$ 1"), p, engine.Battlefield)
	c := g.Card(id)
	if n := c.AbilityCount(); n != 1 {
		t.Fatalf("AbilityCount = %d, want 1", n)
	}
	if ab, ok := c.AbilityAt(0); !ok || ab.Name != "GainLife" {
		t.Errorf("AbilityAt(0) = %v, %v, want the GainLife line", ab, ok)
	}
	if _, ok := c.AbilityAt(1); ok {
		t.Error("AbilityAt(1) found an ability the card does not have")
	}
}
