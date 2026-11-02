package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
	"github.com/jczastkiewicz/crucible/internal/mana"
)

// Crackling Spellslinger's Effect: "The next instant or sorcery spell you
// cast this turn has storm." The Effect's StaticAbilities$ SVar grants Storm
// on the stack (its AddKeyword$ expanded at load, ADR-0038 decision 4), the
// statics are checked again before the SpellCast triggers (MagicStack.java:
// 349), and ForgetOnCast$ ends the Effect after that one spell
// (EffectEffect.java:253-254).
func TestEffectGrantsStormToTheNextSpellOnly(t *testing.T) {
	t.Parallel()

	g, p, other := newTwoPlayerGameOn(t, scenarioDB(t))
	g.SetTurnState(1, p, engine.Main1)
	c := engine.NewScriptedController()
	cast := func() {
		t.Helper()
		bolt := g.NewCard(corpusCard(t, "Lightning Bolt"), p, engine.Hand)
		g.Player(p).ManaPool.Add(mana.Red, 1)
		c.QueueTargets([]engine.EntityID{engine.PlayerEntity(other)})
		if !g.CastSpell(p, bolt, c) {
			t.Fatalf("cast: %v", g.TakePendingError())
		}
		if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
			t.Fatal(err)
		}
	}
	cast()
	// resolveLine casts its own host spell: two spells before the next bolt.
	resolveLine(t, g, p, c, "DB$ Effect | StaticAbilities$ GrantStorm | ForgetOnCast$ Instant.YouCtrl,Sorcery.YouCtrl",
		"GrantStorm", "Mode$ Continuous | Affected$ Instant.YouCtrl,Sorcery.YouCtrl | AffectedZone$ Stack | AddKeyword$ Storm")
	for range 3 {
		c.QueueConfirmEffect(false) // keep each copy's target
	}
	cast()
	if got := g.Player(other).Life; got != 20-3-9 {
		t.Fatalf("life after the storm bolt = %d, want %d (the bolt and two copies)", got, 20-3-9)
	}
	cast()
	if got := g.Player(other).Life; got != 20-3-9-3 {
		t.Errorf("life after the next bolt = %d, want %d (no storm: the Effect is gone)", got, 20-3-9-3)
	}
}
