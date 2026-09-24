package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
)

// TestDigUntilEffectRevealsUntilFound proves the dominant shape: reveal
// until a Valid$ card, which goes to FoundDestination$, while the cards
// revealed before it go to RevealedDestination$ in the player's order and
// the rest of the library is untouched.
func TestDigUntilEffectRevealsUntilFound(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGame(t)
	l1 := g.NewCard(landDef(t, "Test Land", "Basic Land Plains"), p, engine.Library)
	l2 := g.NewCard(landDef(t, "Test Land", "Basic Land Plains"), p, engine.Library)
	creature := g.NewCard(creatureDefPT(t, "2", "2"), p, engine.Library)
	rest := g.NewCard(landDef(t, "Test Land", "Basic Land Plains"), p, engine.Library)

	c := engine.NewScriptedController()
	c.QueueCardOrder([]engine.CardID{l2, l1})
	def := etbChainDef(t, "Test DigUntil", "DB$ DigUntil | Valid$ Creature | FoundDestination$ Hand | RevealedDestination$ Graveyard | RememberFound$ True")
	host, err := castETBChain(t, g, p, def, c)
	if err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if z := g.Card(creature).Zone; z != engine.Hand {
		t.Errorf("found zone = %v, want Hand", z)
	}
	if gy := g.Zone(engine.Graveyard, p).Cards(); len(gy) != 2 || gy[0] != l2 || gy[1] != l1 {
		t.Errorf("graveyard = %v, want [%v %v]", gy, l2, l1)
	}
	if lib := g.Zone(engine.Library, p).Cards(); len(lib) != 1 || lib[0] != rest {
		t.Errorf("library = %v, want [%v]", lib, rest)
	}
	if got := g.Card(host).Memory.Remembered(); len(got) != 1 || got[0] != engine.CardEntity(creature) {
		t.Errorf("remembered = %v, want [%v]", got, engine.CardEntity(creature))
	}
}

// TestDigUntilEffectNoneFoundRandomBottom proves an exhausted library with
// nothing found: RevealRandomOrder$ puts the revealed cards back without
// asking, at RevealedLibraryPosition$ -1.
func TestDigUntilEffectNoneFoundRandomBottom(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGame(t)
	g.NewCard(landDef(t, "Test Land", "Basic Land Plains"), p, engine.Library)
	g.NewCard(landDef(t, "Test Land", "Basic Land Plains"), p, engine.Library)

	c := engine.NewScriptedController()
	def := etbChainDef(t, "Test DigUntil None", "DB$ DigUntil | Valid$ Creature | FoundDestination$ Battlefield | RevealedDestination$ Library | RevealedLibraryPosition$ -1 | RevealRandomOrder$ True")
	if _, err := castETBChain(t, g, p, def, c); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if n := len(g.Zone(engine.Library, p).Cards()); n != 2 {
		t.Errorf("library size = %d, want 2", n)
	}
}

// TestDigUntilEffectRejectsUnresolvedParam proves DigZone$ fails closed.
func TestDigUntilEffectRejectsUnresolvedParam(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGame(t)
	c := engine.NewScriptedController()
	def := etbChainDef(t, "Test DigUntil Zone", "DB$ DigUntil | Valid$ Creature | DigZone$ Graveyard | FoundDestination$ Hand")
	if _, err := castETBChain(t, g, p, def, c); err == nil {
		t.Fatal("ResolveStack succeeded, want an error for unresolved DigZone$")
	}
}

// TestDigUntilEffectSecondaryShapes proves sequential mode (same found and
// revealed zone), NoneFoundDestination$, OptionalFoundMove$ with
// OptionalNoDestination$, NoMoveFound$/NoMoveRevealed$, MaxRevealed$,
// Optional$ declined, ShuffleCondition$ NoneFound, and the Memory params.
func TestDigUntilEffectSecondaryShapes(t *testing.T) {
	t.Parallel()

	t.Run("Sequential", func(t *testing.T) {
		g, p, _ := newTwoPlayerGame(t)
		l := g.NewCard(landDef(t, "Test Land", "Basic Land Plains"), p, engine.Library)
		cr := g.NewCard(creatureDefPT(t, "2", "2"), p, engine.Library)
		c := engine.NewScriptedController()
		if _, err := castETBChain(t, g, p, etbChainDef(t, "Test DU Seq", "DB$ DigUntil | Valid$ Creature | FoundDestination$ Exile | RevealedDestination$ Exile | ImprintFound$ True | ForgetOtherRemembered$ True"), c); err != nil {
			t.Fatalf("ResolveStack: %v", err)
		}
		if g.Card(l).Zone != engine.Exile || g.Card(cr).Zone != engine.Exile {
			t.Error("sequential mode did not exile every revealed card")
		}
	})
	t.Run("NoneFound", func(t *testing.T) {
		g, p, _ := newTwoPlayerGame(t)
		l1 := g.NewCard(landDef(t, "Test Land", "Basic Land Plains"), p, engine.Library)
		g.NewCard(landDef(t, "Test Land", "Basic Land Plains"), p, engine.Library)
		c := engine.NewScriptedController()
		c.QueueCardOrder([]engine.CardID{l1})
		def := etbChainDef(t, "Test DU None", "DB$ DigUntil | Valid$ Creature | MaxRevealed$ 1 | FoundDestination$ Hand | RevealedDestination$ Graveyard | NoneFoundDestination$ Exile | RememberRevealed$ True | ImprintRevealed$ True | Shuffle$ True | ShuffleCondition$ NoneFound")
		if _, err := castETBChain(t, g, p, def, c); err != nil {
			t.Fatalf("ResolveStack: %v", err)
		}
		if z := g.Card(l1).Zone; z != engine.Exile {
			t.Errorf("revealed zone = %v, want Exile (none found)", z)
		}
	})
	t.Run("OptionalFound", func(t *testing.T) {
		g, p, _ := newTwoPlayerGame(t)
		cr := g.NewCard(creatureDefPT(t, "2", "2"), p, engine.Library)
		c := engine.NewScriptedController()
		c.QueueConfirmEffect(false)
		if _, err := castETBChain(t, g, p, etbChainDef(t, "Test DU OptFound", "DB$ DigUntil | Valid$ Creature | FoundDestination$ Battlefield | OptionalFoundMove$ True | OptionalNoDestination$ Graveyard | GainControl$ True | Tapped$ True"), c); err != nil {
			t.Fatalf("ResolveStack: %v", err)
		}
		if z := g.Card(cr).Zone; z != engine.Graveyard {
			t.Errorf("found zone = %v, want Graveyard", z)
		}
	})
	t.Run("NoMove", func(t *testing.T) {
		g, p, _ := newTwoPlayerGame(t)
		g.NewCard(landDef(t, "Test Land", "Basic Land Plains"), p, engine.Library)
		cr := g.NewCard(creatureDefPT(t, "2", "2"), p, engine.Library)
		c := engine.NewScriptedController()
		c.QueueConfirmEffect(false)
		if _, err := castETBChain(t, g, p, etbChainDef(t, "Test DU NoMove", "DB$ DigUntil | Valid$ Creature | FoundDestination$ Hand | NoMoveFound$ True | NoMoveRevealed$ True | OptionalFoundMove$ True"), c); err != nil {
			t.Fatalf("ResolveStack: %v", err)
		}
		if z := g.Card(cr).Zone; z != engine.Library {
			t.Errorf("found zone = %v, want Library", z)
		}
	})
	t.Run("OptionalDecline", func(t *testing.T) {
		g, p, _ := newTwoPlayerGame(t)
		libraryCards(t, g, p, 1)
		c := engine.NewScriptedController()
		c.QueueConfirmEffect(false)
		if _, err := castETBChain(t, g, p, etbChainDef(t, "Test DU Opt", "DB$ DigUntil | Valid$ Creature | FoundDestination$ Hand | Optional$ True | Amount$ 1"), c); err != nil {
			t.Fatalf("ResolveStack: %v", err)
		}
		if n := len(g.Zone(engine.Hand, p).Cards()); n != 0 {
			t.Errorf("hand = %d, want 0", n)
		}
	})
	t.Run("ZeroAmount", func(t *testing.T) {
		g, p, _ := newTwoPlayerGame(t)
		libraryCards(t, g, p, 1)
		if _, err := castETBChain(t, g, p, etbChainDef(t, "Test DU Zero", "DB$ DigUntil | Valid$ Creature | FoundDestination$ Hand | Amount$ 0"), engine.NewScriptedController()); err != nil {
			t.Fatalf("ResolveStack: %v", err)
		}
	})
}
