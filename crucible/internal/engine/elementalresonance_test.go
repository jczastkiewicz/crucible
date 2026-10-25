package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
)

// Elemental Resonance: at your first main phase, add mana equal to the
// enchanted permanent's mana cost -- its colors, and its generic part as
// colorless.
func TestElementalResonanceAddsTheEnchantedPermanentsManaCost(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGameOn(t, scenarioDB(t))
	libraryCards(t, g, p, 8)
	libraryCards(t, g, g.Players()[1], 8)
	bears := g.NewCard(corpusCard(t, "Grizzly Bears"), p, engine.Battlefield) // {1}{G}
	aura := g.NewCard(corpusCard(t, "Elemental Resonance"), p, engine.Battlefield)
	g.Attach(aura, bears)
	g.SetTurnState(2, p, engine.Untap)
	c := engine.NewScriptedController()
	toStep(t, g, c, p, engine.Main1, nil)
	if got := g.Player(p).ManaPool.Total(); got != 2 {
		t.Errorf("mana in pool = %d, want 2 ({1}{G})", got)
	}
}
