package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
)

// TestActivateManaAbilityAddsLiteralColor proves the corpus's own dominant
// real Produced$ shape -- a literal single color -- adds to the pool and
// taps the source (a mana dork's own "T: Add G." shape), firing both
// "becomes tapped" and "taps for mana" (checkTapsTriggers/
// checkTapsForManaTriggers, trigger.go).
func TestActivateManaAbilityAddsLiteralColor(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p := g.Players()[0]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life, g.Player(g.Players()[1]).Life = 20, 20

	def := creatureDefWithAbility(t, "Test Mana Dork", "AB$ Mana | Cost$ T | Produced$ G")
	dork := g.NewCard(def, p, engine.Battlefield)

	c := engine.NewScriptedController()
	if !g.ActivateManaAbility(p, dork, 0, c) {
		t.Fatal("ActivateManaAbility returned false, want true")
	}
	if !g.Card(dork).Tapped {
		t.Error("source not tapped after a Cost$ T mana ability")
	}
	if got, want := g.Player(p).ManaPool.Breakdown(), ([6]int{0, 0, 0, 0, 1, 0}); got != want {
		t.Errorf("pool breakdown = %v, want one green", got)
	}
}

// TestActivateManaAbilityAddsColorless proves Produced$ C -- the corpus's
// own single largest Produced$ value -- adds colorless mana rather than
// treating "C" as a (non-existent) sixth color.
func TestActivateManaAbilityAddsColorless(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p := g.Players()[0]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life, g.Player(g.Players()[1]).Life = 20, 20

	def := creatureDefWithAbility(t, "Test Mana Rock", "AB$ Mana | Cost$ T | Produced$ C")
	rock := g.NewCard(def, p, engine.Battlefield)

	c := engine.NewScriptedController()
	if !g.ActivateManaAbility(p, rock, 0, c) {
		t.Fatal("ActivateManaAbility returned false, want true")
	}
	if got, want := g.Player(p).ManaPool.Breakdown(), ([6]int{0, 0, 0, 0, 0, 1}); got != want {
		t.Errorf("pool breakdown = %v, want one colorless", got)
	}
}

// TestActivateManaAbilityAmount proves a plain-integer Amount$ multiplies
// the mana produced -- resolveNamedAmount's own plain-digit case
// (pumpAmount's own identical reading, reused).
func TestActivateManaAbilityAmount(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p := g.Players()[0]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life, g.Player(g.Players()[1]).Life = 20, 20

	def := creatureDefWithAbility(t, "Test Mana Amount", "AB$ Mana | Cost$ T | Produced$ C | Amount$ 3")
	rock := g.NewCard(def, p, engine.Battlefield)

	c := engine.NewScriptedController()
	if !g.ActivateManaAbility(p, rock, 0, c) {
		t.Fatal("ActivateManaAbility returned false, want true")
	}
	if got, want := g.Player(p).ManaPool.Breakdown(), ([6]int{0, 0, 0, 0, 0, 3}); got != want {
		t.Errorf("pool breakdown = %v, want three colorless", got)
	}
}

// TestActivateManaAbilitySelfSacCost proves a self-sacrifice cost composes
// here exactly as it does for ActivateAbility (activateability.go):
// sacrificeCards reused wholesale, committed after the tap.
func TestActivateManaAbilitySelfSacCost(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p := g.Players()[0]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life, g.Player(g.Players()[1]).Life = 20, 20

	def := creatureDefWithAbility(t, "Test Mana Sac", "AB$ Mana | Cost$ T Sac<1/CARDNAME> | Produced$ C")
	rock := g.NewCard(def, p, engine.Battlefield)

	c := engine.NewScriptedController()
	if !g.ActivateManaAbility(p, rock, 0, c) {
		t.Fatal("ActivateManaAbility returned false, want true")
	}
	if zone := g.Card(rock).Zone; zone != engine.Graveyard {
		t.Errorf("source zone = %v, want Graveyard", zone)
	}
	if got, want := g.Player(p).ManaPool.Breakdown(), ([6]int{0, 0, 0, 0, 0, 1}); got != want {
		t.Errorf("pool breakdown = %v, want one colorless", got)
	}
}

// TestActivateManaAbilityDeclinesWhenSummonSickWithoutHaste proves CR
// 602.5b/302.6 applies to a mana dork the identical way it applies to any
// other Tap-cost activated ability.
func TestActivateManaAbilityDeclinesWhenSummonSickWithoutHaste(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p := g.Players()[0]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life, g.Player(g.Players()[1]).Life = 20, 20

	def := creatureDefWithAbility(t, "Test Mana Sick", "AB$ Mana | Cost$ T | Produced$ G")
	dork := g.NewCard(def, p, engine.Battlefield)
	g.Card(dork).SummonSick = true

	c := engine.NewScriptedController()
	if g.ActivateManaAbility(p, dork, 0, c) {
		t.Error("ActivateManaAbility returned true for a summoning-sick source with no haste, want false")
	}
}

// TestActivateManaAbilityDeclinesForNonManaAPI proves index naming any API
// but "Mana" declines outright -- ActivateManaAbility's own scope is the
// exact complement of ActivateAbility's "API Mana" exclusion
// (activateability.go).
func TestActivateManaAbilityDeclinesForNonManaAPI(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p := g.Players()[0]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life, g.Player(g.Players()[1]).Life = 20, 20

	def := creatureDefWithAbility(t, "Test Not Mana", "AB$ Pump | Cost$ T | Defined$ Self | NumAtt$ 1 | NumDef$ 1")
	creature := g.NewCard(def, p, engine.Battlefield)

	c := engine.NewScriptedController()
	if g.ActivateManaAbility(p, creature, 0, c) {
		t.Error("ActivateManaAbility returned true for API Pump, want false")
	}
}

// TestActivateManaAbilityDeclinesForChosenColor proves Produced$ Any -- CR
// 605.3b's own "choose a color" shape, the corpus's own second-largest
// Produced$ value -- declines rather than guessing a color, since this port
// has no PlayerController hook to ask with yet.
func TestActivateManaAbilityDeclinesForChosenColor(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p := g.Players()[0]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life, g.Player(g.Players()[1]).Life = 20, 20

	def := creatureDefWithAbility(t, "Test Any Color", "AB$ Mana | Cost$ T | Produced$ Any")
	rock := g.NewCard(def, p, engine.Battlefield)

	c := engine.NewScriptedController()
	if g.ActivateManaAbility(p, rock, 0, c) {
		t.Error("ActivateManaAbility returned true for Produced$ Any, want false")
	}
}

// TestActivateManaAbilityDeclinesForBlockedParam proves a param past
// manaAbilityAllowedParams -- RestrictValid$ here, a mana-pool spending
// restriction this port's own Pool cannot tag mana with -- declines the
// whole line rather than producing unrestricted mana (PORT-8/GO-7).
func TestActivateManaAbilityDeclinesForBlockedParam(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p := g.Players()[0]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life, g.Player(g.Players()[1]).Life = 20, 20

	def := creatureDefWithAbility(t, "Test Restricted Mana",
		"AB$ Mana | Cost$ T | Produced$ R | RestrictValid$ Spell.Creature")
	rock := g.NewCard(def, p, engine.Battlefield)

	c := engine.NewScriptedController()
	if g.ActivateManaAbility(p, rock, 0, c) {
		t.Error("ActivateManaAbility returned true with RestrictValid$ present, want false")
	}
}
