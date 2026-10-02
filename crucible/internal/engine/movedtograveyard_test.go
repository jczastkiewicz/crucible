package engine_test

import (
	"strings"
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
)

// TestMovedToGraveyardReplacementSkipsDiesTriggers proves a card exiled
// instead of dying does not die: Rest in Peace exiles Zulaport Cutthroat, so
// its own "whenever this or another creature you control dies" trigger never
// fires (the life totals stay put).
func TestMovedToGraveyardReplacementSkipsDiesTriggers(t *testing.T) {
	t.Parallel()

	g, p, opp := newTwoPlayerGameOn(t, scenarioDB(t))
	g.NewCard(corpusCard(t, "Rest in Peace"), opp, engine.Battlefield)
	cutthroat := g.NewCard(corpusCard(t, "Zulaport Cutthroat"), p, engine.Battlefield)
	g.Card(cutthroat).Damage.Mark(1, false)

	engine.CheckStateBasedActions(g, engine.NewScriptedController())

	if got := g.Card(cutthroat).Zone; got != engine.Exile {
		t.Errorf("Cutthroat zone = %v, want Exile", got)
	}
	if g.Player(p).Life != 20 || g.Player(opp).Life != 20 {
		t.Errorf("life %d/%d, want 20/20: the creature was exiled, not killed", g.Player(p).Life, g.Player(opp).Life)
	}
}

// TestMovedToGraveyardReplacementExilesASpell proves the replacement reaches
// every graveyard move, not only a permanent's: an Instant that resolves under
// Rest in Peace is exiled instead.
func TestMovedToGraveyardReplacementExilesASpell(t *testing.T) {
	t.Parallel()

	g, p, opp := newTwoPlayerGameOn(t, scenarioDB(t))
	g.NewCard(corpusCard(t, "Rest in Peace"), opp, engine.Battlefield)
	spell := g.NewCard(instantDefWithAbility(t, "Test Gain", "0", "SP$ GainLife | Defined$ You | LifeAmount$ 3"), p, engine.Hand)
	c := engine.NewScriptedController()
	if !g.CastSpell(p, spell, c) {
		t.Fatal("CastSpell failed")
	}
	if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if got := g.Card(spell).Zone; got != engine.Exile {
		t.Errorf("spell zone = %v, want Exile", got)
	}
}

// TestMovedToGraveyardReplacementWithAnUnmodelledShapeSetsAnError proves a
// replacement this port cannot apply fails loudly rather than being skipped:
// Kalitas, Traitor of Ghet chains a SubAbility$ (a Zombie token), so the
// creature still dies and the game carries a pending error naming it.
func TestMovedToGraveyardReplacementWithAnUnmodelledShapeSetsAnError(t *testing.T) {
	t.Parallel()

	g, p, opp := newTwoPlayerGameOn(t, scenarioDB(t))
	g.NewCard(corpusCard(t, "Kalitas, Traitor of Ghet"), opp, engine.Battlefield)
	victim := g.NewCard(corpusCard(t, "Grizzly Bears"), p, engine.Battlefield)
	g.Card(victim).Damage.Mark(2, false)

	engine.CheckStateBasedActions(g, engine.NewScriptedController())

	err := g.TakePendingError()
	if err == nil || !strings.Contains(err.Error(), "Kalitas") {
		t.Errorf("pending error = %v, want one naming Kalitas", err)
	}
	if got := g.Card(victim).Zone; got != engine.Graveyard {
		t.Errorf("victim zone = %v, want Graveyard: the replacement was not applied", got)
	}
}
