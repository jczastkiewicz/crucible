package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
)

// Amped Raptor and Bismuth Mindrender: a PlayCost$ past mana replaces the
// mana cost, and its ConvertedManaCost is the card played's mana value.
func TestPlayCostPaidInLifeOrCards(t *testing.T) {
	t.Parallel()

	t.Run("life equal to the mana value", func(t *testing.T) {
		t.Parallel()

		g, p, _ := newTwoPlayerGame(t)
		spell := g.NewCard(gainInstant(t, "Gain", "2 W", "5"), p, engine.Exile)
		c := engine.NewScriptedController()
		c.QueueConfirmEffect(true)
		host := pushPlayLine(t, g, p, nil, "DB$ Play | Defined$ Remembered | ValidSA$ Spell | PlayCost$ PayLife<ConvertedManaCost> | Optional$ True")
		if err := resolvePlay(t, g, host, c, spell); err != nil {
			t.Fatal(err)
		}
		if got := g.Player(p).Life; got != 22 {
			t.Errorf("life = %d, want 20 - 3 + 5", got)
		}
	})
	t.Run("a discarded card", func(t *testing.T) {
		t.Parallel()

		g, p, _ := newTwoPlayerGame(t)
		spell := g.NewCard(gainInstant(t, "Gain", "2 W", "5"), p, engine.Exile)
		fodder := g.NewCard(creatureDefPT(t, "1", "1"), p, engine.Hand)
		c := engine.NewScriptedController()
		c.QueueConfirmEffect(true)
		c.QueueDiscardChoice([]engine.CardID{fodder})
		host := pushPlayLine(t, g, p, nil, "DB$ Play | Defined$ Remembered | ValidSA$ Spell | PlayCost$ Discard<1/Card> | Optional$ True")
		if err := resolvePlay(t, g, host, c, spell); err != nil {
			t.Fatal(err)
		}
		if got := g.Card(fodder).Zone; got != engine.Graveyard {
			t.Errorf("the discarded card is in %v, want the graveyard", got)
		}
		if got := g.Player(p).Life; got != 25 {
			t.Errorf("life = %d, want 25", got)
		}
	})
	t.Run("mana beside it", func(t *testing.T) {
		t.Parallel()

		g, p, _ := newTwoPlayerGame(t)
		spell := g.NewCard(gainInstant(t, "Gain", "2 W", "5"), p, engine.Exile)
		c := engine.NewScriptedController()
		c.QueueConfirmEffect(true)
		host := pushPlayLine(t, g, p, nil, "DB$ Play | Defined$ Remembered | ValidSA$ Spell | PlayCost$ 1 PayLife<2> | Optional$ True")
		if err := resolvePlay(t, g, host, c, spell); err == nil {
			t.Error("a PlayCost$ with mana and life resolved")
		}
	})
}
