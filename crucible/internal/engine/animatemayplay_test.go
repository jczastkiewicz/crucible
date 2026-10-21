package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
	"github.com/jczastkiewicz/crucible/internal/mana"
)

// TestAnimatePerpetualMayPlayGrantsAnAlternativeCost proves Animate
// staticAbilities$ with a Mode$ Continuous | MayPlay$ line (Flames of
// Moradin, Mine Security): the target card, still in hand, gains "you may pay
// {R} rather than this spell's mana cost" and can be cast for it (the card
// costs {3} and the pool holds one red mana).
func TestAnimatePerpetualMayPlayGrantsAnAlternativeCost(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGame(t)
	pricey := creatureDefPT(t, "2", "2")
	pricey.Faces[0].ManaCost = mana.MustParse("3")
	target := g.NewCard(pricey, p, engine.Hand)
	def := etbChainDef(t, "Test Perpetual Discount",
		"DB$ Animate | ValidTgts$ Card.YouOwn | Duration$ Perpetual | staticAbilities$ PerpAltCost",
		"PerpAltCost", "Mode$ Continuous | EffectZone$ All | MayPlay$ True | MayPlayAltManaCost$ R | MayPlayDontGrantZonePermissions$ True | Affected$ Card.Self | AffectedZone$ Hand,Graveyard,Library,Exile,Command")
	c := engine.NewScriptedController()
	c.QueueTargets([]engine.EntityID{engine.CardEntity(target)})
	if _, err := castETBChain(t, g, p, def, c); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	c.QueueOption(1)
	g.Player(p).ManaPool.Add(mana.Red, 1)
	if !g.CastSpell(p, target, c) {
		t.Fatalf("CastSpell for {R} failed: %v", g.TakePendingError())
	}
}
