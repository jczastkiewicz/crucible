package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
	"github.com/jczastkiewicz/crucible/internal/mana"
)

// Dracoplasm: as it enters, sacrifice any number of creatures; its power and
// toughness become their totals. The sacrifice runs before it lands (it cannot
// sacrifice itself), the Animate after.
func TestDracoplasmTakesTheTotalOfTheSacrificedCreatures(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGameOn(t, scenarioDB(t))
	c := engine.NewScriptedController()
	g.SetTurnState(1, p, engine.Main1)
	a := g.NewCard(creatureDefPT(t, "3", "1"), p, engine.Battlefield)
	b := g.NewCard(creatureDefPT(t, "2", "4"), p, engine.Battlefield)
	spell := g.NewCard(corpusCard(t, "Dracoplasm"), p, engine.Hand)
	g.Player(p).ManaPool.Add(mana.Blue, 3)
	g.Player(p).ManaPool.Add(mana.Red, 3)
	c.QueueConfirmEffect(true)
	c.QueueCardChoice([]engine.CardID{a, b})
	if !g.CastSpell(p, spell, c) {
		t.Fatalf("CastSpell failed: %v", g.TakePendingError())
	}
	if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
		t.Fatalf("ResolveStack: %v (pending %v)", err, g.TakePendingError())
	}
	pw, _ := g.Card(spell).Power()
	tg, _ := g.Card(spell).Toughness()
	if pw != 5 || tg != 5 {
		t.Errorf("Dracoplasm is %d/%d, want 5/5", pw, tg)
	}
}
