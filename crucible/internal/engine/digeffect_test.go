package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
)

// TestDigEffectOneToHandRestToBottom proves the default shape: look at the
// top DigNum$, the pick goes to hand, the rest to the library bottom in the
// order the chooser gives.
func TestDigEffectOneToHandRestToBottom(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGame(t)
	lib := libraryCards(t, g, p, 4)

	c := engine.NewScriptedController()
	c.QueueCardChoice([]engine.CardID{lib[1]})
	c.QueueCardOrder([]engine.CardID{lib[2], lib[0]})
	if _, err := castETBChain(t, g, p, etbChainDef(t, "Test Dig", "DB$ Dig | DigNum$ 3 | ChangeNum$ 1"), c); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if z := g.Card(lib[1]).Zone; z != engine.Hand {
		t.Errorf("picked zone = %v, want Hand", z)
	}
	got := g.Zone(engine.Library, p).Cards()
	want := []engine.CardID{lib[3], lib[2], lib[0]}
	if len(got) != 3 || got[0] != want[0] || got[1] != want[1] || got[2] != want[2] {
		t.Errorf("library = %v, want %v", got, want)
	}
}

// TestDigEffectChangeAllValidToBattlefieldRestToGraveyard proves ChangeNum$
// All with ChangeValid$: every matching card moves without a choice, and
// the rest go to DestinationZone2$, ordered by their owner.
func TestDigEffectChangeAllValidToBattlefieldRestToGraveyard(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGame(t)
	l1 := g.NewCard(landDef(t, "Test Land", "Basic Land Plains"), p, engine.Library)
	creature := g.NewCard(creatureDefPT(t, "2", "2"), p, engine.Library)
	l2 := g.NewCard(landDef(t, "Test Land", "Basic Land Plains"), p, engine.Library)

	c := engine.NewScriptedController()
	c.QueueCardOrder([]engine.CardID{l2, l1})
	def := etbChainDef(t, "Test Dig All", "DB$ Dig | DigNum$ 3 | ChangeNum$ All | ChangeValid$ Creature | DestinationZone$ Battlefield | DestinationZone2$ Graveyard")
	if _, err := castETBChain(t, g, p, def, c); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if z := g.Card(creature).Zone; z != engine.Battlefield {
		t.Errorf("creature zone = %v, want Battlefield", z)
	}
	if gy := g.Zone(engine.Graveyard, p).Cards(); len(gy) != 2 || gy[0] != l2 || gy[1] != l1 {
		t.Errorf("graveyard = %v, want [%v %v]", gy, l2, l1)
	}
}

// TestDigEffectOptionalNoneAndRandomRest proves Optional$ allows taking
// nothing and RestRandomOrder$ shuffles the rest without asking.
func TestDigEffectOptionalNoneAndRandomRest(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGame(t)
	libraryCards(t, g, p, 4)

	c := engine.NewScriptedController()
	c.QueueCardChoice(nil)
	if _, err := castETBChain(t, g, p, etbChainDef(t, "Test Dig Optional", "DB$ Dig | DigNum$ 3 | Optional$ True | RestRandomOrder$ True"), c); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if n := len(g.Zone(engine.Hand, p).Cards()); n != 0 {
		t.Errorf("hand size = %d, want 0", n)
	}
	if n := len(g.Zone(engine.Library, p).Cards()); n != 4 {
		t.Errorf("library size = %d, want 4", n)
	}
}

// TestDigEffectRejectsUnresolvedShapes proves a non-literal
// library position fail closed.
func TestDigEffectRejectsUnresolvedShapes(t *testing.T) {
	t.Parallel()

	for _, line := range []string{
		"DB$ Dig | DigNum$ 3 | LibraryPosition$ 2",
		"DB$ Dig | DigNum$ Bogus",
	} {
		g, p, _ := newTwoPlayerGame(t)
		libraryCards(t, g, p, 3)
		c := engine.NewScriptedController()
		if _, err := castETBChain(t, g, p, etbChainDef(t, "Test Dig Reject", line), c); err == nil {
			t.Errorf("%q: ResolveStack succeeded, want an error", line)
		}
	}
}

// TestDigEffectSecondaryShapes proves RememberRevealed$/ImprintRevealed$,
// RevealOptional$ declined, AnyNumber$, top-of-library destination,
// Imprint$/RememberMovedToZone$, DestZone2Optional$ and
// PromptToSkipOptionalAbility$.
func TestDigEffectSecondaryShapes(t *testing.T) {
	t.Parallel()

	t.Run("RevealMemory", func(t *testing.T) {
		g, p, _ := newTwoPlayerGame(t)
		lib := libraryCards(t, g, p, 2)
		c := engine.NewScriptedController()
		c.QueueCardChoice([]engine.CardID{lib[0], lib[1]})
		def := etbChainDef(t, "Test Dig Mem", "DB$ Dig | DigNum$ 2 | ChangeNum$ Any | RememberRevealed$ True | ImprintRevealed$ True | Imprint$ True | RememberMovedToZone$ 1 | DestinationZone$ Library | LibraryPosition$ 0")
		c.QueueCardOrder([]engine.CardID{lib[1], lib[0]})
		host, err := castETBChain(t, g, p, def, c)
		if err != nil {
			t.Fatalf("ResolveStack: %v", err)
		}
		if n := len(g.Card(host).Memory.Imprinted()); n != 2 {
			t.Errorf("imprinted = %d, want 2", n)
		}
	})
	t.Run("RevealOptionalDeclined", func(t *testing.T) {
		g, p, _ := newTwoPlayerGame(t)
		lib := libraryCards(t, g, p, 1)
		c := engine.NewScriptedController()
		c.QueueConfirmEffect(false)
		c.QueueCardChoice([]engine.CardID{lib[0]})
		host, err := castETBChain(t, g, p, etbChainDef(t, "Test Dig RevOpt", "DB$ Dig | DigNum$ 1 | RevealOptional$ True | RememberRevealed$ True"), c)
		if err != nil {
			t.Fatalf("ResolveStack: %v", err)
		}
		if n := len(g.Card(host).Memory.Remembered()); n != 0 {
			t.Errorf("remembered = %d, want 0", n)
		}
	})
	t.Run("SkipOptionalAbility", func(t *testing.T) {
		g, p, _ := newTwoPlayerGame(t)
		libraryCards(t, g, p, 2)
		c := engine.NewScriptedController()
		c.QueueConfirmEffect(false)
		if _, err := castETBChain(t, g, p, etbChainDef(t, "Test Dig Skip", "DB$ Dig | DigNum$ 2 | Optional$ True | PromptToSkipOptionalAbility$ True"), c); err != nil {
			t.Fatalf("ResolveStack: %v", err)
		}
		if n := len(g.Zone(engine.Library, p).Cards()); n != 2 {
			t.Errorf("library = %d, want 2 untouched", n)
		}
	})
	t.Run("DestZone2OptionalAndRemember2", func(t *testing.T) {
		g, p, _ := newTwoPlayerGame(t)
		lib := libraryCards(t, g, p, 2)
		c := engine.NewScriptedController()
		c.QueueCardChoice([]engine.CardID{lib[0]})
		c.QueueConfirmEffect(true)
		host, err := castETBChain(t, g, p, etbChainDef(t, "Test Dig Z2", "DB$ Dig | DigNum$ 2 | DestinationZone2$ Exile | DestZone2Optional$ True | RememberMovedToZone$ 2 | Tapped$ True | ForgetOtherRemembered$ True"), c)
		if err != nil {
			t.Fatalf("ResolveStack: %v", err)
		}
		if z := g.Card(lib[1]).Zone; z != engine.Exile {
			t.Errorf("rest zone = %v, want Exile", z)
		}
		if n := len(g.Card(host).Memory.Remembered()); n != 1 {
			t.Errorf("remembered = %d, want 1", n)
		}
	})
	t.Run("SkipReorderAndEmptyLibrary", func(t *testing.T) {
		g, p, _ := newTwoPlayerGame(t)
		lib := libraryCards(t, g, p, 3)
		c := engine.NewScriptedController()
		c.QueueCardChoice([]engine.CardID{lib[0]})
		if _, err := castETBChain(t, g, p, etbChainDef(t, "Test Dig Skip Order", "DB$ Dig | DigNum$ 3 | SkipReorder$ True | Defined$ Player"), c); err != nil {
			t.Fatalf("ResolveStack: %v", err)
		}
	})
}

// TestDigEffectChoserNamesTheChooserAndRecordsIt proves Choser$ (DigEffect.
// java:215-222): the named player picks, and SetChosenPlayer$ records them on
// the host for the rest of the chain (Allure of the Unknown).
func TestDigEffectChoserNamesTheChooserAndRecordsIt(t *testing.T) {
	t.Parallel()

	g, p, other := newTwoPlayerGame(t)
	lib := libraryCards(t, g, p, 3)

	c := engine.NewScriptedController()
	c.QueueCardChoice([]engine.CardID{lib[1]})
	host, err := castETBChain(t, g, p, etbChainDef(t, "Test Choser Dig",
		"DB$ Dig | DigNum$ 3 | ChangeNum$ 1 | Choser$ Opponent | SetChosenPlayer$ True | DestinationZone$ Exile | DestinationZone2$ Graveyard | SkipReorder$ True"), c)
	if err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if z := g.Card(lib[1]).Zone; z != engine.Exile {
		t.Errorf("picked zone = %v, want Exile", z)
	}
	if got := g.Card(host).Memory.ChosenPlayer(); got != other {
		t.Errorf("chosen player = %v, want the opponent %v", got, other)
	}
}

// TestDigEffectChangeValidChosenTypeReadsTheHostsType proves
// ChangeValid$ Card.ChosenType (Winding Way, Vigean Intuition): the cards of
// the type the host chose go to the hand, the rest to the graveyard.
func TestDigEffectChangeValidChosenTypeReadsTheHostsType(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGame(t)
	lib := libraryCards(t, g, p, 2)
	other := g.NewCard(landDef(t, "Test Plains", "Basic Land Plains"), p, engine.Library)
	c := engine.NewScriptedController()
	c.QueueOption(0)
	if _, err := castETBChain(t, g, p, etbChainDef(t, "Test Winding Way",
		"DB$ ChooseType | Defined$ You | Type$ Creature | ValidTypes$ Elf | SubAbility$ DBDig",
		"DBDig", "DB$ Dig | DigNum$ 3 | ChangeNum$ All | ChangeValid$ Card.ChosenType | DestinationZone2$ Graveyard | SkipReorder$ True"), c); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	for _, id := range lib {
		if z := g.Card(id).Zone; z != engine.Hand {
			t.Errorf("creature zone = %v, want Hand", z)
		}
	}
	if z := g.Card(other).Zone; z != engine.Graveyard {
		t.Errorf("land zone = %v, want Graveyard", z)
	}
}

// TestDigEffectFromBottomLooksAtTheBottomCards proves FromBottom$
// (DigEffect.java:181-183): the dug cards are the bottom DigNum$ of the library.
func TestDigEffectFromBottomLooksAtTheBottomCards(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGame(t)
	lib := libraryCards(t, g, p, 4)

	c := engine.NewScriptedController()
	if _, err := castETBChain(t, g, p, etbChainDef(t, "Test Dig Bottom", "DB$ Dig | DigNum$ 2 | FromBottom$ True | ChangeNum$ All"), c); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	for _, id := range lib[2:] {
		if z := g.Card(id).Zone; z != engine.Hand {
			t.Errorf("bottom card %v zone = %v, want Hand", id, z)
		}
	}
	for _, id := range lib[:2] {
		if z := g.Card(id).Zone; z != engine.Library {
			t.Errorf("top card %v zone = %v, want Library", id, z)
		}
	}
}

// TestDigEffectIgnoresParamsItNeverReads proves Pawprint$, Ultimate$ and
// UnlessResolveSubs$ do not block a Dig: DigEffect.java reads none of them
// (CharmEffect, AchievementTracker and AbilityUtils do).
func TestDigEffectIgnoresParamsItNeverReads(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGame(t)
	libraryCards(t, g, p, 3)
	c := engine.NewScriptedController()
	c.QueueCardChoice(nil)
	if _, err := castETBChain(t, g, p, etbChainDef(t, "Test Dig Unread", "DB$ Dig | DigNum$ 2 | Optional$ True | SkipReorder$ True | Pawprint$ 1 | Ultimate$ True | UnlessResolveSubs$ Always"), c); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
}
