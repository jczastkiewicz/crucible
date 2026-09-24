package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
)

// TestChangeZoneAllEffectBouncesEveryMatch proves the sweep: every matching
// permanent moves, the owner orders the two headed to their hand, and a
// non-match stays.
func TestChangeZoneAllEffectBouncesEveryMatch(t *testing.T) {
	t.Parallel()

	g, p, other := newTwoPlayerGame(t)
	a := g.NewCard(creatureDefPT(t, "2", "2"), other, engine.Battlefield)
	b := g.NewCard(creatureDefPT(t, "2", "2"), other, engine.Battlefield)
	mine := g.NewCard(creatureDefPT(t, "2", "2"), p, engine.Battlefield)

	c := engine.NewScriptedController()
	c.QueueCardOrder([]engine.CardID{b, a})
	host, err := castETBChain(t, g, p, etbChainDef(t, "Test Sweep", "DB$ ChangeZoneAll | ChangeType$ Creature.OppCtrl | Origin$ Battlefield | Destination$ Hand | RememberChanged$ True"), c)
	if err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if hand := g.Zone(engine.Hand, other).Cards(); len(hand) != 2 || hand[0] != b || hand[1] != a {
		t.Errorf("opponent hand = %v, want [%v %v]", hand, b, a)
	}
	if z := g.Card(mine).Zone; z != engine.Battlefield {
		t.Errorf("own creature zone = %v, want Battlefield", z)
	}
	if n := len(g.Card(host).Memory.Remembered()); n != 2 {
		t.Errorf("remembered %d, want 2", n)
	}
}

// TestChangeZoneAllEffectShuffleSkipsOrdering proves Shuffle$: cards headed
// into a library that is shuffled afterwards need no ordering decision.
func TestChangeZoneAllEffectShuffleSkipsOrdering(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGame(t)
	g.NewCard(creatureDefPT(t, "2", "2"), p, engine.Graveyard)
	g.NewCard(creatureDefPT(t, "2", "2"), p, engine.Graveyard)

	c := engine.NewScriptedController()
	def := etbChainDef(t, "Test Elixir", "DB$ ChangeZoneAll | ChangeType$ Card | Defined$ You | Origin$ Graveyard | Destination$ Library | Shuffle$ True")
	if _, err := castETBChain(t, g, p, def, c); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if n := len(g.Zone(engine.Library, p).Cards()); n != 2 {
		t.Errorf("library size = %d, want 2", n)
	}
	if n := len(g.Zone(engine.Graveyard, p).Cards()); n != 0 {
		t.Errorf("graveyard size = %d, want 0", n)
	}
}

// TestChangeZoneAllEffectRandomOrderAndLibraryBottom proves RandomOrder$ uses
// the game's RNG instead of asking, and LibraryPosition$ -1 appends.
func TestChangeZoneAllEffectRandomOrderAndLibraryBottom(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGame(t)
	top := libraryCards(t, g, p, 1)[0]
	g.NewCard(creatureDefPT(t, "2", "2"), p, engine.Hand)
	g.NewCard(creatureDefPT(t, "2", "2"), p, engine.Hand)

	c := engine.NewScriptedController()
	def := etbChainDef(t, "Test Bottom", "DB$ ChangeZoneAll | ChangeType$ Creature | Defined$ You | Origin$ Hand | Destination$ Library | LibraryPosition$ -1 | RandomOrder$ True")
	if _, err := castETBChain(t, g, p, def, c); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	lib := g.Zone(engine.Library, p).Cards()
	if len(lib) != 3 || lib[0] != top {
		t.Errorf("library = %v, want 3 cards with %v still on top", lib, top)
	}
}

// TestChangeZoneAllEffectRejectsUnresolvedParam proves TypeLimit$ fails closed.
func TestChangeZoneAllEffectRejectsUnresolvedParam(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGame(t)
	c := engine.NewScriptedController()
	def := etbChainDef(t, "Test Limit", "DB$ ChangeZoneAll | ChangeType$ Creature | Origin$ Battlefield | Destination$ Hand | TypeLimit$ 1")
	if _, err := castETBChain(t, g, p, def, c); err == nil {
		t.Fatal("ResolveStack succeeded, want an error for unresolved TypeLimit$")
	}
}

// TestChangeZoneAllEffectSecondaryShapes proves DefinedPlayer$ ordering into
// a library, Optional$ declined, and the battlefield destination's
// GainControl$/Tapped$/Imprint$/ForgetChanged$.
func TestChangeZoneAllEffectSecondaryShapes(t *testing.T) {
	t.Parallel()

	t.Run("LibraryOrderedByDefinedPlayer", func(t *testing.T) {
		g, p, other := newTwoPlayerGame(t)
		a := g.NewCard(creatureDefPT(t, "2", "2"), other, engine.Hand)
		b := g.NewCard(creatureDefPT(t, "2", "2"), other, engine.Hand)
		c := engine.NewScriptedController()
		c.QueueCardOrder([]engine.CardID{a, b})
		def := etbChainDef(t, "Test All Lib", "DB$ ChangeZoneAll | ChangeType$ Card | Defined$ Opponent | DefinedPlayer$ You | Origin$ Hand | Destination$ Library")
		if _, err := castETBChain(t, g, p, def, c); err != nil {
			t.Fatalf("ResolveStack: %v", err)
		}
		if lib := g.Zone(engine.Library, other).Cards(); len(lib) != 2 || lib[0] != b {
			t.Errorf("library = %v, want %v on top", lib, b)
		}
	})
	t.Run("OptionalDecline", func(t *testing.T) {
		g, p, other := newTwoPlayerGame(t)
		a := g.NewCard(creatureDefPT(t, "2", "2"), other, engine.Battlefield)
		c := engine.NewScriptedController()
		c.QueueConfirmEffect(false)
		if _, err := castETBChain(t, g, p, etbChainDef(t, "Test All Opt", "DB$ ChangeZoneAll | ChangeType$ Creature.OppCtrl | Origin$ Battlefield | Destination$ Exile | Optional$ True"), c); err != nil {
			t.Fatalf("ResolveStack: %v", err)
		}
		if z := g.Card(a).Zone; z != engine.Battlefield {
			t.Errorf("zone = %v, want Battlefield", z)
		}
	})
	t.Run("ReanimateAll", func(t *testing.T) {
		g, p, other := newTwoPlayerGame(t)
		a := g.NewCard(creatureDefPT(t, "2", "2"), other, engine.Graveyard)
		c := engine.NewScriptedController()
		def := etbChainDef(t, "Test All Reanimate", "DB$ ChangeZoneAll | ChangeType$ Creature | Origin$ Graveyard | Destination$ Battlefield | GainControl$ True | Tapped$ True | Imprint$ True | ForgetChanged$ True | RememberChanged$ Creature | ForgetOtherRemembered$ True")
		host, err := castETBChain(t, g, p, def, c)
		if err != nil {
			t.Fatalf("ResolveStack: %v", err)
		}
		card := g.Card(a)
		if card.Zone != engine.Battlefield || card.Controller() != p || !card.Tapped {
			t.Errorf("zone %v controller %v tapped %v", card.Zone, card.Controller(), card.Tapped)
		}
		if n := len(g.Card(host).Memory.Imprinted()); n != 1 {
			t.Errorf("imprinted = %d, want 1", n)
		}
	})
}
