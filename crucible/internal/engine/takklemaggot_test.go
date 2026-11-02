package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
)

// Takklemaggot with no creature to enchant returns as a non-Aura
// enchantment: its Animate runs on the card in the graveyard and survives the
// move to the battlefield, because Java's changeZone keeps the same object for
// a battlefield destination (GameAction.java:172) instead of copying it
// (GameAction.java:207). It then deals 1 damage at that player's upkeep.
func TestTakklemaggotReturnsAsANonAuraEnchantment(t *testing.T) {
	t.Parallel()

	g, p, other := newTwoPlayerGameOn(t, scenarioDB(t))
	g.SetTurnState(1, p, engine.Main1)
	bears := g.NewCard(corpusCard(t, "Grizzly Bears"), other, engine.Battlefield)
	maggot := g.NewCard(corpusCard(t, "Takklemaggot"), p, engine.Battlefield)
	g.Attach(maggot, bears)
	sba(g)
	c := engine.NewScriptedController()
	c.QueueCardChoice(nil) // no creature Takklemaggot could enchant
	if _, err := resolveNow(t, g, p, c, []engine.EntityID{engine.CardEntity(bears)},
		"DB$ Destroy | ValidTgts$ Creature"); err != nil {
		t.Fatal(err)
	}
	if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
		t.Fatal(err)
	}
	m := g.Card(maggot)
	if m.Zone != engine.Battlefield || m.Type().HasSubtype("Aura") {
		t.Fatalf("Takklemaggot in %v with types %v, want a non-Aura enchantment on the battlefield", m.Zone, m.Type())
	}
	life := g.Player(other).Life
	advanceUntil(t, g, c, func() bool { return g.Turn() == 2 && g.ActivePhase() == engine.Draw })
	if got := life - g.Player(other).Life; got != 1 {
		t.Errorf("%d damage at that player's upkeep, want 1", got)
	}
}
