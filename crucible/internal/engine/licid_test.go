package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/cardtype"
	"github.com/jczastkiewicz/crucible/internal/engine"
	"github.com/jczastkiewicz/crucible/internal/mana"
)

// Quickening Licid: its own ability becomes an Aura (RemoveThisAbility$,
// Duration$ Permanent) with the revert ability RevertCost$ grants; paying {W}
// ends the effect and gives the printed ability back
// (AnimateEffectBase.java:86-88, :205-219).
func TestLicidBecomesAnAuraAndRevertsForItsRevertCost(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGameOn(t, scenarioDB(t))
	g.SetTurnState(1, p, engine.Main1)
	licid := g.NewCard(corpusCard(t, "Quickening Licid"), p, engine.Battlefield)
	g.Card(licid).SummonSick = false
	target := g.NewCard(creatureDefPT(t, "2", "2"), p, engine.Battlefield)

	c := engine.NewScriptedController()
	c.QueueTargets([]engine.EntityID{engine.CardEntity(target)})
	g.Player(p).ManaPool.Add(mana.White, 2)
	c.QueuePayGeneric(mana.ShardW)
	if !g.ActivateAbility(p, licid, 0, c) {
		t.Fatal("ActivateAbility(Licid) = false")
	}
	if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if !g.Card(licid).Type().Has(cardtype.Enchantment) {
		t.Fatalf("Licid types = %v, want an Enchantment", g.Card(licid).Type())
	}
	if g.ActivateAbility(p, licid, 0, c) {
		t.Error("the printed ability is still available on the Aura")
	}

	g.Player(p).ManaPool.Add(mana.White, 1)
	c.QueuePayGeneric(mana.ShardW)
	if !g.ActivateAbility(p, licid, 1, c) {
		t.Fatal("the revert ability cannot be activated")
	}
	if g.StackLen() != 0 {
		t.Errorf("the revert went on the stack (%d items), want immediate", g.StackLen())
	}
	engine.CheckStateBasedActions(g, c)
	if g.Card(licid).Type().Has(cardtype.Enchantment) {
		t.Errorf("Licid is still an Aura after the revert")
	}
}

// Arcane Lighthouse's AnimateAll CantHaveKeyword$ (AnimateEffectBase.java:
// 139-141): the opponent's creatures lose hexproof and cannot have it, even
// from a later grant.
func TestAnimateAllCantHaveKeywordStripsLaterGrants(t *testing.T) {
	t.Parallel()

	g, p, other := newTwoPlayerGame(t)
	bear := g.NewCard(creatureDefPTKeywords(t, "2", "2", "Hexproof"), other, engine.Battlefield)
	c := engine.NewScriptedController()
	if _, err := resolveNow(t, g, p, c, nil, "DB$ AnimateAll | ValidCards$ Creature.OppCtrl | RemoveKeywords$ Hexproof & Shroud | CantHaveKeyword$ Hexproof & Shroud"); err != nil {
		t.Fatal(err)
	}
	if _, err := resolveNow(t, g, other, c, []engine.EntityID{engine.CardEntity(bear)}, "DB$ Pump | ValidTgts$ Creature | KW$ Hexproof"); err != nil {
		t.Fatal(err)
	}
	engine.CheckStateBasedActions(g, c)
	if g.Card(bear).HasKeyword("Hexproof") {
		t.Error("the creature has Hexproof despite CantHaveKeyword$")
	}
}
