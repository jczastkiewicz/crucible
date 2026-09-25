package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
)

// deckCards puts n vanilla cards into p's zone z, top to bottom in the order
// given, and returns their CardIDs in that same top-to-bottom order --
// libraryCards' own shape (milleffect_test.go) for a non-Library deck zone.
func deckCards(t *testing.T, g *engine.Game, p engine.PlayerID, z engine.ZoneType, n int) []engine.CardID {
	t.Helper()
	ids := make([]engine.CardID, n)
	for i := 0; i < n; i++ {
		ids[i] = g.NewCard(creatureDefPT(t, "1", "1"), p, z)
	}
	return ids
}

// TestOpenAttractionEffectDefaultMovesTopCardToBattlefield proves
// OpenAttraction's own dominant real shape (bare, no params): the ability's
// own Controller (Defined$'s "You" default) puts the top card of their own
// Attraction deck onto the battlefield.
func TestOpenAttractionEffectDefaultMovesTopCardToBattlefield(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGame(t)
	deck := deckCards(t, g, p, engine.AttractionDeck, 2)

	resolveLine(t, g, p, engine.NewScriptedController(), "DB$ OpenAttraction")

	if z := g.Card(deck[0]).Zone; z != engine.Battlefield {
		t.Errorf("top card zone = %v, want Battlefield", z)
	}
	if z := g.Card(deck[1]).Zone; z != engine.AttractionDeck {
		t.Errorf("second card zone = %v, want still AttractionDeck", z)
	}
	if c := g.Card(deck[0]).Controller(); c != p {
		t.Errorf("opened card controller = %v, want %v", c, p)
	}
}

// TestOpenAttractionEffectAmountAndDefinedAndRemember proves Amount$
// (Lang.getNumeral's own "two" corpus lines), Defined$ (a different
// player's own deck) and Remember$ (source.Memory) together: the second
// most common real shape after the bare line.
func TestOpenAttractionEffectAmountAndDefinedAndRemember(t *testing.T) {
	t.Parallel()

	g, p, other := newTwoPlayerGame(t)
	deck := deckCards(t, g, p, engine.AttractionDeck, 3)
	otherDeck := deckCards(t, g, other, engine.AttractionDeck, 1)

	host := resolveLine(t, g, p, engine.NewScriptedController(),
		"DB$ OpenAttraction | Amount$ 2 | Defined$ You | Remember$ True")

	for i := 0; i < 2; i++ {
		if z := g.Card(deck[i]).Zone; z != engine.Battlefield {
			t.Errorf("card %d zone = %v, want Battlefield", i, z)
		}
	}
	if z := g.Card(deck[2]).Zone; z != engine.AttractionDeck {
		t.Errorf("third card zone = %v, want still AttractionDeck", z)
	}
	if z := g.Card(otherDeck[0]).Zone; z != engine.AttractionDeck {
		t.Errorf("other's card zone = %v, want untouched AttractionDeck", z)
	}
	remembered := g.Card(host).Memory.Remembered()
	if len(remembered) != 2 {
		t.Fatalf("remembered = %v, want 2 entries", remembered)
	}
}

// TestOpenAttractionEffectEmptyDeckIsANoOp proves an empty Attraction deck
// (GetPlayerCardsIn's own empty read) stops the loop rather than erroring --
// OpenAttractionEffect.java's own `if (attractionDeck.isEmpty()) continue`.
func TestOpenAttractionEffectEmptyDeckIsANoOp(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGame(t)
	resolveLine(t, g, p, engine.NewScriptedController(), "DB$ OpenAttraction | Amount$ 3")
}

// TestOpenAttractionEffectConditionFalseSkips proves subAbilityConditionMet's
// own false branch: a ConditionPhases$ that does not match the current phase
// leaves the Attraction deck untouched.
func TestOpenAttractionEffectConditionFalseSkips(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGame(t)
	deck := deckCards(t, g, p, engine.AttractionDeck, 1)

	resolveLine(t, g, p, engine.NewScriptedController(), "DB$ OpenAttraction | ConditionPhases$ Bogus")

	if z := g.Card(deck[0]).Zone; z != engine.AttractionDeck {
		t.Errorf("zone = %v, want still AttractionDeck", z)
	}
}

// TestOpenAttractionEffectSkipsPlayersWhoHaveLost proves the identical
// `if (!p.isInGame()) continue` every other Defined$ walk in this port
// already carries: a player who has lost never opens an Attraction, even
// when Defined$ names them.
func TestOpenAttractionEffectSkipsPlayersWhoHaveLost(t *testing.T) {
	t.Parallel()

	g, p, other := newTwoPlayerGame(t)
	deck := deckCards(t, g, other, engine.AttractionDeck, 1)
	g.Player(other).Lost = true

	if _, err := resolveNow(t, g, p, engine.NewScriptedController(), nil,
		"DB$ OpenAttraction | Defined$ Opponent"); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}

	if z := g.Card(deck[0]).Zone; z != engine.AttractionDeck {
		t.Errorf("zone = %v, want still AttractionDeck (opponent has lost)", z)
	}
}

// TestOpenAttractionEffectRejectsUnresolvedDefinedAndAmount proves the two
// error returns targetedOrDefinedPlayers and optionalAmount can each give
// this effect reach the caller unchanged (GO-7), the same "don't guess"
// contract every other Defined$/Amount$ reader in this port already has.
func TestOpenAttractionEffectRejectsUnresolvedDefinedAndAmount(t *testing.T) {
	t.Parallel()

	cases := []string{
		"DB$ OpenAttraction | Defined$ Bogus",
		"DB$ OpenAttraction | Amount$ Bogus",
	}
	for _, trig := range cases {
		g, p, _ := newTwoPlayerGame(t)
		deckCards(t, g, p, engine.AttractionDeck, 1)
		if _, err := resolveNow(t, g, p, engine.NewScriptedController(), nil, trig); err == nil {
			t.Errorf("%q: want an error, got nil", trig)
		}
	}
}

// TestAssembleContraptionEffectConditionFalseSkips is
// TestOpenAttractionEffectConditionFalseSkips' own AssembleContraption
// sibling.
func TestAssembleContraptionEffectConditionFalseSkips(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGame(t)
	deck := deckCards(t, g, p, engine.ContraptionDeck, 1)

	resolveLine(t, g, p, engine.NewScriptedController(), "DB$ AssembleContraption | ConditionPhases$ Bogus")

	if z := g.Card(deck[0]).Zone; z != engine.ContraptionDeck {
		t.Errorf("zone = %v, want still ContraptionDeck", z)
	}
}

// TestAssembleContraptionEffectDefaultMovesTopCardAndDialsSprocket proves
// AssembleContraption's own dominant real shape: the ability's own
// Controller puts the top card of their own Contraption deck onto the
// battlefield and dials it to a chosen sprocket (1-3).
func TestAssembleContraptionEffectDefaultMovesTopCardAndDialsSprocket(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGame(t)
	deck := deckCards(t, g, p, engine.ContraptionDeck, 2)

	c := engine.NewScriptedController()
	c.QueueNumberChoice(2)
	resolveLine(t, g, p, c, "DB$ AssembleContraption")

	if z := g.Card(deck[0]).Zone; z != engine.Battlefield {
		t.Errorf("top contraption zone = %v, want Battlefield", z)
	}
	if s := g.Card(deck[0]).Sprocket; s != 2 {
		t.Errorf("sprocket = %d, want 2", s)
	}
	if z := g.Card(deck[1]).Zone; z != engine.ContraptionDeck {
		t.Errorf("second contraption zone = %v, want still ContraptionDeck", z)
	}
}

// TestAssembleContraptionEffectAmountAndRemember proves Amount$ (a plain
// integer) assembling several Contraptions in one resolution, each dialed
// independently, with Remember$ recording every one.
func TestAssembleContraptionEffectAmountAndRemember(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGame(t)
	deck := deckCards(t, g, p, engine.ContraptionDeck, 2)

	c := engine.NewScriptedController()
	c.QueueNumberChoice(1)
	c.QueueNumberChoice(3)
	host := resolveLine(t, g, p, c, "DB$ AssembleContraption | Amount$ 2 | Remember$ True")

	if s := g.Card(deck[0]).Sprocket; s != 1 {
		t.Errorf("first sprocket = %d, want 1", s)
	}
	if s := g.Card(deck[1]).Sprocket; s != 3 {
		t.Errorf("second sprocket = %d, want 3", s)
	}
	remembered := g.Card(host).Memory.Remembered()
	if len(remembered) != 2 {
		t.Fatalf("remembered = %v, want 2 entries", remembered)
	}
}

// TestAssembleContraptionEffectSprocketClearsOnLeavingBattlefield proves the
// Sprocket reset Game.Move's own battlefield-leaving branch carries: a
// Contraption sacrificed or destroyed forgets its dial (CR 725.4a) rather
// than remembering it for a future reassembly.
func TestAssembleContraptionEffectSprocketClearsOnLeavingBattlefield(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGame(t)
	deckCards(t, g, p, engine.ContraptionDeck, 1)

	c := engine.NewScriptedController()
	c.QueueNumberChoice(1)
	host := resolveLine(t, g, p, c, "DB$ AssembleContraption | Remember$ True")
	remembered := g.Card(host).Memory.Remembered()
	if len(remembered) != 1 {
		t.Fatalf("remembered = %v, want 1 entry", remembered)
	}
	contraption, ok := remembered[0].AsCard()
	if !ok {
		t.Fatalf("remembered entry %v is not a card", remembered[0])
	}
	if s := g.Card(contraption).Sprocket; s != 1 {
		t.Fatalf("sprocket before destroy = %d, want 1", s)
	}
	g.Move(contraption, engine.Graveyard, g.Card(contraption).Owner)
	if s := g.Card(contraption).Sprocket; s != 0 {
		t.Errorf("sprocket after destroy = %d, want 0 (cleared)", s)
	}
}

// TestAssembleContraptionEffectRejectsUnresolvedParams proves DefinedContraption$,
// Reassemble$, an unresolved DefinedAssembler$ and Amount$ Result all fail
// closed (GO-7) rather than silently reassembling the wrong Contraption or
// assembling the wrong number.
func TestAssembleContraptionEffectRejectsUnresolvedParams(t *testing.T) {
	t.Parallel()

	cases := []string{
		"DB$ AssembleContraption | DefinedContraption$ Self",
		"DB$ AssembleContraption | Reassemble$ True",
		"DB$ AssembleContraption | DefinedAssembler$ ReplacedCause",
		"DB$ AssembleContraption | Amount$ Result",
	}
	for _, trig := range cases {
		g, p, _ := newTwoPlayerGame(t)
		deckCards(t, g, p, engine.ContraptionDeck, 1)
		if _, err := resolveNow(t, g, p, engine.NewScriptedController(), nil, trig); err == nil {
			t.Errorf("%q: want a rejected-param error, got nil", trig)
		}
	}
}
