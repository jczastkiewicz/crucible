package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
)

// TestRegenerateEffectShieldReplacesDestroy proves CR 701.15b: a shielded
// creature that would be destroyed is tapped and stays, and the shield is
// spent so a second destroy kills it.
func TestRegenerateEffectShieldReplacesDestroy(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGame(t)
	c := engine.NewScriptedController()
	def := etbChainDef(t, "Test Regen",
		"DB$ Regenerate | Defined$ Self | SubAbility$ DBDestroy",
		"DBDestroy", "DB$ Destroy | Defined$ Self | SubAbility$ DBDestroy2",
		"DBDestroy2", "DB$ Destroy | Defined$ Self")
	host, err := castETBChain(t, g, p, def, c)
	if err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if z := g.Card(host).Zone; z != engine.Graveyard {
		t.Errorf("host zone = %v, want Graveyard after the second destroy", z)
	}
}

// TestRegenerateEffectSurvivesOneDestroyTapped proves the survivor's state
// after one destroy: tapped, shield spent.
func TestRegenerateEffectSurvivesOneDestroyTapped(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGame(t)
	c := engine.NewScriptedController()
	def := etbChainDef(t, "Test Regen Once", "DB$ Regenerate | Defined$ Self | SubAbility$ DBDestroy",
		"DBDestroy", "DB$ Destroy | Defined$ Self")
	host, err := castETBChain(t, g, p, def, c)
	if err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	card := g.Card(host)
	if card.Zone != engine.Battlefield || !card.Tapped || card.RegenShields != 0 {
		t.Errorf("host zone %v tapped %v shields %d, want Battlefield true 0", card.Zone, card.Tapped, card.RegenShields)
	}
}

// TestRegenerateEffectNoRegenIgnoresShield proves NoRegen$ destroys through
// a shield.
func TestRegenerateEffectNoRegenIgnoresShield(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGame(t)
	c := engine.NewScriptedController()
	def := etbChainDef(t, "Test Regen NoRegen", "DB$ Regenerate | Defined$ Self | SubAbility$ DBDestroy",
		"DBDestroy", "DB$ Destroy | Defined$ Self | NoRegen$ True")
	host, err := castETBChain(t, g, p, def, c)
	if err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if z := g.Card(host).Zone; z != engine.Graveyard {
		t.Errorf("host zone = %v, want Graveyard", z)
	}
}

// TestRegenerateEffectSavesFromLethalDamage proves the state-based lethal
// damage destruction is regenerable too: the damage is removed and the
// creature stays.
func TestRegenerateEffectSavesFromLethalDamage(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGame(t)
	c := engine.NewScriptedController()
	def := etbChainDef(t, "Test Regen Damage", "DB$ Regenerate | Defined$ Self | SubAbility$ DBHit",
		"DBHit", "DB$ DealDamage | Defined$ Self | NumDmg$ 5")
	host, err := castETBChain(t, g, p, def, c)
	if err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	card := g.Card(host)
	if card.Zone != engine.Battlefield || card.Damage.Marked != 0 {
		t.Errorf("host zone %v damage %d, want Battlefield 0", card.Zone, card.Damage.Marked)
	}
}

// TestRegenerateEffectShieldEndsAtCleanup proves an unused shield expires at
// the cleanup step.
func TestRegenerateEffectShieldEndsAtCleanup(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGame(t)
	c := engine.NewScriptedController()
	host, err := castETBChain(t, g, p, etbChainDef(t, "Test Regen Cleanup", "DB$ Regenerate | Defined$ Self"), c)
	if err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if n := g.Card(host).RegenShields; n != 1 {
		t.Fatalf("shields = %d, want 1", n)
	}
	g.SetTurnState(1, p, engine.EndOfTurn)
	g.AdvancePhase(c)
	if n := g.Card(host).RegenShields; n != 0 {
		t.Errorf("shields after cleanup = %d, want 0", n)
	}
}
