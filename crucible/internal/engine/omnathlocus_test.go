package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
	"github.com/jczastkiewicz/crucible/internal/mana"
)

// Omnath, Locus of All: reveal a top card with three or more colored mana
// symbols to add three mana in any combination of its colors, then put it into
// your hand.
func TestOmnathAddsThreeManaInTheRevealedCardsColors(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGameOn(t, scenarioDB(t))
	g.NewCard(corpusCard(t, "Omnath, Locus of All"), p, engine.Battlefield)
	ult := g.NewCard(corpusCard(t, "Cruel Ultimatum"), p, engine.Library) // {U}{U}{B}{B}{R}{R}
	g.SetTurnState(1, p, engine.Untap)
	c := engine.NewScriptedController()
	c.QueueConfirmReveal(true)
	c.QueueManaColor(mana.Blue)
	c.QueueManaColor(mana.Black)
	c.QueueManaColor(mana.Red)
	toStep(t, g, c, p, engine.Main1, nil)
	if got := g.Player(p).ManaPool.Total(); got != 3 {
		t.Errorf("mana in pool = %d, want 3", got)
	}
	if z := g.Card(ult).Zone; z != engine.Hand {
		t.Errorf("the revealed card is in %v, want Hand", z)
	}
}
