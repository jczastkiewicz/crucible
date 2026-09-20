package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/carddb/compile"
	"github.com/jczastkiewicz/crucible/internal/cardtype"
	"github.com/jczastkiewicz/crucible/internal/engine"
	"github.com/jczastkiewicz/crucible/internal/mana"
)

// landDef builds just enough of a *compile.Card for Card.Type() to answer
// "does this have basic land type X" -- the only thing TapLandForMana reads
// off a card's definition. typeLine can name more than one basic land type
// (a dual-typed land such as a Snow-Covered Plains Island keeps two separate
// intrinsic abilities).
func landDef(t *testing.T, name, typeLine string) *compile.Card {
	t.Helper()
	def := &compile.Card{Name: name}
	def.Faces[0].Type = cardtype.Parse(attachmentTypeRegistry(t), typeLine)
	return def
}

// Tapping a Plains for white is CR 305.6's baseline case: the land taps and
// the pool gains exactly the color the type line names.
func TestTapLandForManaAddsColorAndTaps(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a")
	p := g.Players()[0]
	plains := g.NewCard(landDef(t, "Plains", "Basic Land Plains"), p, engine.Battlefield)

	if !g.TapLandForMana(p, plains, mana.White, engine.NewScriptedController()) {
		t.Fatal("TapLandForMana failed tapping a Plains for white")
	}
	if !g.Card(plains).Tapped {
		t.Error("Plains not tapped after TapLandForMana")
	}
	if got := g.Player(p).ManaPool.Breakdown(); got != [6]int{1, 0, 0, 0, 0, 0} {
		t.Errorf("pool = %v, want one white", got)
	}
}

// A land already tapped has nothing left to tap -- CR 302.6's tap cost
// cannot be paid twice.
func TestTapLandForManaFailsWhenAlreadyTapped(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a")
	p := g.Players()[0]
	plains := g.NewCard(landDef(t, "Plains", "Basic Land Plains"), p, engine.Battlefield)
	g.Card(plains).Tapped = true

	if g.TapLandForMana(p, plains, mana.White, engine.NewScriptedController()) {
		t.Fatal("TapLandForMana succeeded on an already-tapped land")
	}
	if got := g.Player(p).ManaPool.Total(); got != 0 {
		t.Errorf("pool total after a failed tap = %d, want 0", got)
	}
}

// Only the controller may activate the land's own mana ability.
func TestTapLandForManaFailsWhenNotControlled(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	owner, other := g.Players()[0], g.Players()[1]
	plains := g.NewCard(landDef(t, "Plains", "Basic Land Plains"), owner, engine.Battlefield)

	if g.TapLandForMana(other, plains, mana.White, engine.NewScriptedController()) {
		t.Fatal("TapLandForMana succeeded for a player who does not control the land")
	}
	if got := g.Player(other).ManaPool.Total(); got != 0 {
		t.Errorf("non-controller's pool total = %d, want 0", got)
	}
}

// A land still in hand has no mana ability to activate -- CR 605's mana
// abilities are activated by permanents on the battlefield.
func TestTapLandForManaFailsWhenNotOnBattlefield(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a")
	p := g.Players()[0]
	plains := g.NewCard(landDef(t, "Plains", "Basic Land Plains"), p, engine.Hand)

	if g.TapLandForMana(p, plains, mana.White, engine.NewScriptedController()) {
		t.Fatal("TapLandForMana succeeded on a land still in hand")
	}
}

// Asking an Island for white fails: the land's own basic land type is the
// only color its intrinsic ability produces.
func TestTapLandForManaFailsWhenLandLacksThatColor(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a")
	p := g.Players()[0]
	island := g.NewCard(landDef(t, "Island", "Basic Land Island"), p, engine.Battlefield)

	if g.TapLandForMana(p, island, mana.White, engine.NewScriptedController()) {
		t.Fatal("TapLandForMana succeeded asking an Island for white")
	}
	if g.Card(island).Tapped {
		t.Error("Island tapped despite the failed request")
	}
}

// A land with two basic land types keeps two separate intrinsic abilities
// (CR 305.6), but tapping is one shared cost: activating either one taps the
// card, so the other is no longer available afterward.
func TestTapLandForManaSupportsDualBasicLandType(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a")
	p := g.Players()[0]
	dual := g.NewCard(landDef(t, "Dual", "Basic Land Plains Island"), p, engine.Battlefield)

	if !g.TapLandForMana(p, dual, mana.Blue, engine.NewScriptedController()) {
		t.Fatal("TapLandForMana failed tapping a Plains Island for blue")
	}
	if g.TapLandForMana(p, dual, mana.White, engine.NewScriptedController()) {
		t.Fatal("TapLandForMana succeeded tapping an already-tapped dual land again")
	}
	if got := g.Player(p).ManaPool.Breakdown(); got != [6]int{0, 1, 0, 0, 0, 0} {
		t.Errorf("pool = %v, want one blue", got)
	}
}

// A land carrying the Snow supertype produces snow mana (CR 106.3a) --
// nothing about the ability itself differs from a plain basic land's, only
// which of Pool's two buckets for that color receives it.
func TestTapLandForManaProducesSnowManaFromASnowLand(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a")
	p := g.Players()[0]
	snowPlains := g.NewCard(landDef(t, "Snow-Covered Plains", "Basic Snow Land Plains"), p, engine.Battlefield)

	if !g.TapLandForMana(p, snowPlains, mana.White, engine.NewScriptedController()) {
		t.Fatal("TapLandForMana failed tapping a Snow-Covered Plains for white")
	}
	if got, want := g.Player(p).ManaPool.SnowBreakdown(), [6]int{1, 0, 0, 0, 0, 0}; got != want {
		t.Errorf("SnowBreakdown() = %v, want %v (the mana produced is snow)", got, want)
	}
	if got, want := g.Player(p).ManaPool.Breakdown(), [6]int{1, 0, 0, 0, 0, 0}; got != want {
		t.Errorf("Breakdown() = %v, want %v", got, want)
	}
}

// color must be exactly one of the five basic colors -- the zero value or
// more than one bit set is an engine invariant breach, not something a
// fixture or a card can cause, so it panics rather than silently reporting
// false (GO-7).
func TestTapLandForManaRequiresExactlyOneBasicColor(t *testing.T) {
	t.Parallel()

	defer func() {
		if recover() == nil {
			t.Error("TapLandForMana(White|Blue) did not panic")
		}
	}()

	g := newGame(t, "a")
	p := g.Players()[0]
	plains := g.NewCard(landDef(t, "Plains", "Basic Land Plains"), p, engine.Battlefield)
	g.TapLandForMana(p, plains, mana.White|mana.Blue, engine.NewScriptedController())
}
