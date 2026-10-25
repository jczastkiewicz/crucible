package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
)

// Tibalt, Cosmic Impostor: "as it enters, you get an emblem". The Effect in the
// replacement carries StaticAbilities$ of its own, which belong to the emblem
// and are not part of the chain that must run before the move.
func TestAsItEntersEmblemEffectDoesNotBlockTheEntry(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGame(t)
	g.SetTurnState(1, p, engine.Main1)
	def := scriptDef(t, "Emblem Walker", "Creature Elf",
		"K:ETBReplacement:Other:Emblem",
		"SVar:Emblem:DB$ Effect | Name$ Emblem - Test | StaticAbilities$ Play | Duration$ Permanent",
		"SVar:Play:Mode$ Continuous | MayPlay$ True | Affected$ Card.ExiledWithEffectSource | AffectedZone$ Exile")
	card := g.NewCard(def, p, engine.Hand)
	c := engine.NewScriptedController()
	if !g.CastSpell(p, card, c) {
		t.Fatalf("CastSpell failed: %v", g.TakePendingError())
	}
	if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
		t.Fatalf("ResolveStack: %v (pending %v)", err, g.TakePendingError())
	}
	if z := g.Card(card).Zone; z != engine.Battlefield {
		t.Errorf("the card is in %v, want Battlefield", z)
	}
	if n := len(g.Zone(engine.Command, p).Cards()); n != 1 {
		t.Errorf("command zone holds %d cards, want the one emblem", n)
	}
}
