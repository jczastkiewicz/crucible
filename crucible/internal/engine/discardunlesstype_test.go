package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
)

// Thirst for Knowledge: "discard two cards unless you discard an artifact
// card" -- the pick is two cards, or one matching card.
func TestDiscardUnlessTypeAcceptsTwoCardsOrOneMatching(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name     string
		pick     func(art, a, b engine.CardID) []engine.CardID
		wantGone int
		wantErr  bool
	}{
		{"one artifact", func(art, _, _ engine.CardID) []engine.CardID { return []engine.CardID{art} }, 1, false},
		{"two cards", func(_, a, b engine.CardID) []engine.CardID { return []engine.CardID{a, b} }, 2, false},
		{"one other card", func(_, a, _ engine.CardID) []engine.CardID { return []engine.CardID{a} }, 0, true},
	} {
		g, p, _ := newTwoPlayerGame(t)
		art := g.NewCard(artifactDef(t), p, engine.Hand)
		a := g.NewCard(creatureDefPT(t, "1", "1"), p, engine.Hand)
		b := g.NewCard(creatureDefPT(t, "1", "1"), p, engine.Hand)
		c := engine.NewScriptedController()
		picked := tc.pick(art, a, b)
		c.QueueCardChoice(picked)
		c.QueueCardOrder(picked)
		_, err := discardNow(t, g, p, c, nil, "DB$ Discard | Defined$ You | Mode$ TgtChoose | NumCards$ 2 | UnlessType$ Artifact")
		if (err != nil) != tc.wantErr {
			t.Errorf("%s: err = %v, want error %v", tc.name, err, tc.wantErr)
			continue
		}
		gone := 0
		for _, id := range []engine.CardID{art, a, b} {
			if g.Card(id).Zone == engine.Graveyard {
				gone++
			}
		}
		if gone != tc.wantGone {
			t.Errorf("%s: %d cards discarded, want %d", tc.name, gone, tc.wantGone)
		}
	}
}

// An empty hand asks nobody.
func TestDiscardUnlessTypeWithAnEmptyHandDiscardsNothing(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGame(t)
	if _, err := discardNow(t, g, p, engine.NewScriptedController(), nil,
		"DB$ Discard | Defined$ You | Mode$ TgtChoose | NumCards$ 2 | UnlessType$ Artifact"); err != nil {
		t.Fatal(err)
	}
}

// With no card of the type in hand the full count is the only selection.
func TestDiscardUnlessTypeWithoutAMatchDiscardsTheCount(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGame(t)
	a := g.NewCard(creatureDefPT(t, "1", "1"), p, engine.Hand)
	b := g.NewCard(creatureDefPT(t, "1", "1"), p, engine.Hand)
	c := engine.NewScriptedController()
	c.QueueCardChoice([]engine.CardID{a, b})
	c.QueueCardOrder([]engine.CardID{a, b})
	if _, err := discardNow(t, g, p, c, nil, "DB$ Discard | Defined$ You | Mode$ TgtChoose | NumCards$ 2 | UnlessType$ Artifact"); err != nil {
		t.Fatal(err)
	}
	if g.Card(a).Zone != engine.Graveyard || g.Card(b).Zone != engine.Graveyard {
		t.Error("the two cards were not discarded")
	}
}
