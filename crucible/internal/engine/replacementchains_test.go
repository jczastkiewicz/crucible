package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
)

// These tests cover a Replace* ReplaceWith$ head that chains a SubAbility$:
// ReplacementHandler resolves the whole ability, so the chain runs once the
// head has changed the event (AbilityUtils.java:1302-1330), a DamageDone
// chain reading the damage prevented (ReplacementHandler.java:600-668).

// TestShieldEffectExilesItselfAfterPreventing proves a one-shot prevention
// effect ("the next time ... prevent 2 of that damage", Forcefield's and
// Dark Sphere's shape) prevents once and then exiles itself, so the second
// hit is dealt in full.
func TestShieldEffectExilesItselfAfterPreventing(t *testing.T) {
	t.Parallel()

	g, p, other := newTwoPlayerGame(t)
	g.SetTurnState(1, p, engine.Main1)
	ctl := engine.NewScriptedController()
	if _, err := resolveNow(t, g, p, ctl, nil, "DB$ Effect | ReplacementEffects$ RPrevent",
		"RPrevent", "Event$ DamageDone | ValidTarget$ You | ReplaceWith$ Prevent | PreventionEffect$ True | Description$ Prevent 2.",
		"Prevent", "DB$ ReplaceDamage | Amount$ 2 | SubAbility$ ExileEffect",
		"ExileEffect", "DB$ ChangeZone | Defined$ Self | Origin$ Command | Destination$ Exile"); err != nil {
		t.Fatalf("Effect: %v", err)
	}
	if got := len(g.Zone(engine.Command, p).Cards()); got != 1 {
		t.Fatalf("command zone holds %d cards, want the effect", got)
	}
	for i, want := range []int{1, 3} {
		life := g.Player(p).Life
		if _, err := resolveNow(t, g, other, ctl, nil, "DB$ DealDamage | Defined$ Opponent | NumDmg$ 3"); err != nil {
			t.Fatalf("hit %d: %v", i+1, err)
		}
		if got := life - g.Player(p).Life; got != want {
			t.Errorf("hit %d: lost %d, want %d", i+1, got, want)
		}
	}
	if got := len(g.Zone(engine.Command, p).Cards()); got != 0 {
		t.Errorf("command zone holds %d cards after the shield was used, want 0", got)
	}
}

// TestRockHydraTradesCountersForDamage proves Rock Hydra prevents damage up
// to its +1/+1 counters and its chained RemoveCounter takes off one counter
// per point prevented.
func TestRockHydraTradesCountersForDamage(t *testing.T) {
	t.Parallel()

	g, p, other := newTwoPlayerGameOn(t, scenarioDB(t))
	g.SetTurnState(1, p, engine.Main1)
	hydra := g.NewCard(corpusCard(t, "Rock Hydra"), p, engine.Battlefield)
	g.Card(hydra).Counters.Add(engine.P1P1, 5)
	if _, err := resolveNow(t, g, other, engine.NewScriptedController(), []engine.EntityID{engine.CardEntity(hydra)},
		"DB$ DealDamage | ValidTgts$ Creature | NumDmg$ 3"); err != nil {
		t.Fatalf("DealDamage: %v", err)
	}
	if got := g.Card(hydra).Counters.Count(engine.P1P1); got != 2 {
		t.Errorf("+1/+1 counters = %d, want 2", got)
	}
	if got := g.Card(hydra).Damage.Marked; got != 0 {
		t.Errorf("damage marked = %d, want 0", got)
	}
}

// TestMeliraAllowsOnePoisonCounterPerTurn proves Melira, the Living Cure's
// replacement: three poison counters become one, and its reflexive effect
// stops any more that turn.
func TestMeliraAllowsOnePoisonCounterPerTurn(t *testing.T) {
	t.Parallel()

	g, p, other := newTwoPlayerGameOn(t, scenarioDB(t))
	g.SetTurnState(1, p, engine.Main1)
	g.NewCard(corpusCard(t, "Melira, the Living Cure"), other, engine.Battlefield)
	ctl := engine.NewScriptedController()
	for i, num := range []string{"3", "2"} {
		if _, err := resolveNow(t, g, p, ctl, nil, "DB$ Poison | Defined$ Opponent | Num$ "+num); err != nil {
			t.Fatalf("poison %d: %v", i+1, err)
		}
		if got := g.Player(other).Counters.Count(engine.Poison); got != 1 {
			t.Errorf("after poison %d: %d poison counters, want 1", i+1, got)
		}
	}
}

// TestDividedRedirectShieldSendsTheNextTwoDamage proves Harm's Way's shape:
// a "next 2 damage" ReplaceSplitDamage with DivideShield$ deals 2 of a 3
// damage hit to its remembered target instead, then is spent, so a second
// hit is dealt in full.
func TestDividedRedirectShieldSendsTheNextTwoDamage(t *testing.T) {
	t.Parallel()

	g, p, other := newTwoPlayerGame(t)
	g.SetTurnState(1, p, engine.Main1)
	ctl := engine.NewScriptedController()
	if _, err := resolveNow(t, g, p, ctl, []engine.EntityID{engine.PlayerEntity(other)},
		"DB$ Effect | ValidTgts$ Any | ReplacementEffects$ ReDamage | RememberObjects$ Targeted",
		"ReDamage", "Event$ DamageDone | ActiveZones$ Command | ValidTarget$ You,Permanent.YouCtrl | ReplaceWith$ SplitDamage | DamageTarget$ Remembered | Description$ Redirect.",
		"SplitDamage", "DB$ ReplaceSplitDamage | DamageTarget$ Remembered | VarName$ ShieldAmount | DivideShield$ True",
		"ShieldAmount", "Number$2"); err != nil {
		t.Fatalf("Effect: %v", err)
	}
	for i, want := range [][2]int{{1, 2}, {3, 0}} {
		mine, theirs := g.Player(p).Life, g.Player(other).Life
		if _, err := resolveNow(t, g, other, ctl, nil, "DB$ DealDamage | Defined$ Opponent | NumDmg$ 3"); err != nil {
			t.Fatalf("hit %d: %v", i+1, err)
		}
		if got := [2]int{mine - g.Player(p).Life, theirs - g.Player(other).Life}; got != want {
			t.Errorf("hit %d: (p lost, other lost) = %v, want %v", i+1, got, want)
		}
	}
}
