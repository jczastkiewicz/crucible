package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
)

// StaticAbilityContinuous.java:357-363, 842-851: AddStaticAbility$ gives each
// affected card a static ability of its own, which then applies like a printed
// one. A banner granting "this creature gets +2/+0" pumps every creature, and
// stops the pass the banner leaves.
func TestContinuousAddStaticAbilityGrantsAContinuousEffect(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGame(t)
	banner := g.NewCard(scriptDef(t, "Test Banner", "Enchantment",
		"S:Mode$ Continuous | Affected$ Creature.YouCtrl | AddStaticAbility$ STPump",
		"SVar:STPump:Mode$ Continuous | Affected$ Card.Self | AddPower$ 2"), p, engine.Battlefield)
	bears := g.NewCard(creatureDefPT(t, "2", "2"), p, engine.Battlefield)
	sba(g)

	if got, _ := g.Card(bears).Power(); got != 4 {
		t.Errorf("power = %d, want 4 -- the granted static should apply to its new host", got)
	}
	g.Move(banner, engine.Graveyard, p)
	sba(g)
	if got, _ := g.Card(bears).Power(); got != 2 {
		t.Errorf("power after the grant ended = %d, want 2", got)
	}
}

// GameAction.java:1152-1168: a static granted in Layer 6 applies in that layer
// at once, so its own AddKeyword$ lands, and in the layers after it (Rune of
// Might on an Equipment: "equipped creature gets +1/+1 and has trample").
func TestContinuousGrantedStaticAppliesItsKeywordInTheGrantingLayer(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGame(t)
	g.NewCard(scriptDef(t, "Test Rune", "Enchantment",
		"S:Mode$ Continuous | Affected$ Creature.YouCtrl | AddStaticAbility$ STBuff",
		"SVar:STBuff:Mode$ Continuous | Affected$ Card.Self | AddPower$ 1 | AddToughness$ 1 | AddKeyword$ Trample"), p, engine.Battlefield)
	bears := g.NewCard(creatureDefPT(t, "2", "2"), p, engine.Battlefield)
	sba(g)

	if !g.Card(bears).HasKeyword("Trample") {
		t.Error("no trample from the granted static")
	}
	wantPT(t, g, bears, 3, 3)
}

// A static a granted static grants applies too.
func TestContinuousGrantedStaticGrantsAStatic(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGame(t)
	g.NewCard(scriptDef(t, "Test Rune", "Enchantment",
		"S:Mode$ Continuous | Affected$ Creature.YouCtrl | AddStaticAbility$ STOuter",
		"SVar:STOuter:Mode$ Continuous | Affected$ Card.Self | AddStaticAbility$ STInner",
		"SVar:STInner:Mode$ Continuous | Affected$ Card.Self | AddKeyword$ Flying"), p, engine.Battlefield)
	bears := g.NewCard(creatureDefPT(t, "2", "2"), p, engine.Battlefield)
	sba(g)

	if !g.Card(bears).HasKeyword("Flying") {
		t.Error("no flying from the doubly granted static")
	}
}

// AddReplacementEffect$ gives each affected card a replacement effect that
// every replacement walk reads: a granted "if a player would gain life, that
// player gains no life instead" stops a gain while the grant lasts.
func TestContinuousAddReplacementEffectGrantsAReplacement(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p := g.Players()[0]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life = 20
	banner := g.NewCard(scriptDef(t, "Test Banner", "Enchantment",
		"S:Mode$ Continuous | Affected$ Creature.YouCtrl | AddReplacementEffect$ REPrevent",
		"SVar:REPrevent:Event$ GainLife | ActiveZones$ Battlefield | Prevent$ True | Description$ No one gains life."), p, engine.Battlefield)
	g.NewCard(creatureDefPT(t, "2", "2"), p, engine.Battlefield)
	sba(g)

	if err := castETBGainLife(t, g, p, etbGainLifeTriggerDefParams(t, "Test GainLife", "Defined$ You | LifeAmount$ 5", nil)); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if got := g.Player(p).Life; got != 20 {
		t.Errorf("life = %d, want 20 -- the granted replacement should stop the gain", got)
	}

	g.Move(banner, engine.Graveyard, p)
	sba(g)
	if err := castETBGainLife(t, g, p, etbGainLifeTriggerDefParams(t, "Test GainLife", "Defined$ You | LifeAmount$ 5", nil)); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if got := g.Player(p).Life; got != 25 {
		t.Errorf("life = %d, want 25 once the grant ended", got)
	}
}

// StaticAbility.java:380: Condition$ Monarch holds while the host's controller
// is the monarch (Dawnglade Regent's "permanents you control have hexproof").
func TestContinuousConditionMonarchFollowsTheDesignation(t *testing.T) {
	t.Parallel()

	g, p, opp := newTwoPlayerGame(t)
	g.NewCard(scriptDef(t, "Test Regent", "Creature",
		"S:Mode$ Continuous | Affected$ Creature.YouCtrl | AddKeyword$ Hexproof | Condition$ Monarch"), p, engine.Battlefield)
	bears := g.NewCard(creatureDefPT(t, "2", "2"), p, engine.Battlefield)
	sba(g)
	if g.Card(bears).HasKeyword("Hexproof") {
		t.Error("hexproof without the monarchy")
	}
	g.SetMonarch(opp)
	sba(g)
	if g.Card(bears).HasKeyword("Hexproof") {
		t.Error("hexproof while the opponent is the monarch")
	}
	g.SetMonarch(p)
	sba(g)
	if !g.Card(bears).HasKeyword("Hexproof") {
		t.Error("no hexproof while the controller is the monarch")
	}
}

// RemoveAllAbilities$ takes away every trait granted before it, statics
// included, but not one granted after it (CardTraitChanges' remove predicate
// runs over the earlier rows only).
func TestContinuousRemoveAllAbilitiesTakesGrantedStatics(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGame(t)
	g.NewCard(scriptDef(t, "Test Banner", "Enchantment",
		"S:Mode$ Continuous | Affected$ Creature.YouCtrl | AddStaticAbility$ STPump",
		"SVar:STPump:Mode$ Continuous | Affected$ Card.Self | AddPower$ 2"), p, engine.Battlefield)
	g.NewCard(scriptDef(t, "Test Humility", "Enchantment",
		"S:Mode$ Continuous | Affected$ Creature | RemoveAllAbilities$ True"), p, engine.Battlefield)
	bears := g.NewCard(creatureDefPT(t, "2", "2"), p, engine.Battlefield)
	sba(g)

	if got, _ := g.Card(bears).Power(); got != 2 {
		t.Errorf("power = %d, want 2 -- the later wipe removes the earlier grant", got)
	}
}
