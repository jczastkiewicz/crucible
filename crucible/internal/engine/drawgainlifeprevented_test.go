package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
)

// TestDrawPreventedByBareReplacement proves drawPrevented (replacement.go)
// is wired into DrawCards (turn.go): possessed_portal.txt's own real
// bare "if a player would draw a card, that player skips that draw
// instead" (Prevent$ True, ValidPlayer$ Player -- every player) stops the
// draw entirely, and the card stays in the library.
func TestDrawPreventedByBareReplacement(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a")
	p := g.Players()[0]
	top := g.NewCard(nil, p, engine.Library)
	g.NewCard(replacementEnchantmentDef(t, "Test Draw Lock",
		"Event$ Draw | ActiveZones$ Battlefield | ValidPlayer$ Player | Prevent$ True | Description$ Skip all draws."), p, engine.Battlefield)

	g.DrawCards(p, 1, engine.NewScriptedController())

	if len(g.Zone(engine.Hand, p).Cards()) != 0 {
		t.Errorf("hand has %d cards, want 0 -- Prevent$ True must stop the draw entirely", len(g.Zone(engine.Hand, p).Cards()))
	}
	if g.Card(top).Zone != engine.Library {
		t.Errorf("card zone = %v, want Library -- the prevented card must never move", g.Card(top).Zone)
	}
}

// TestDrawPreventedDoesNotCauseEmptyLibraryLoss proves the CR-faithful
// ordering drawPrevented's own doc comment names: Player.doDraw's own
// Event$ Draw replacement check runs BEFORE Java ever looks at whether the
// library is empty, so a prevented draw from an empty library never sets
// DrewFromEmptyLibrary -- possessed_portal.txt's own real shield would
// otherwise still lose its own controller to CR 704.5b even though the
// draw it prevented was the only thing that could have exposed the empty
// library at all.
func TestDrawPreventedDoesNotCauseEmptyLibraryLoss(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a")
	p := g.Players()[0]
	g.NewCard(replacementEnchantmentDef(t, "Test Draw Lock",
		"Event$ Draw | ActiveZones$ Battlefield | ValidPlayer$ Player | Prevent$ True | Description$ Skip all draws."), p, engine.Battlefield)

	g.DrawCards(p, 1, engine.NewScriptedController())

	if g.Player(p).DrewFromEmptyLibrary {
		t.Error("DrewFromEmptyLibrary = true, want false -- a prevented draw must never reach the empty-library check")
	}
}

// TestDrawPreventedWhenIsPresentConditionMet proves the general-gate
// fold-in (replacementRequirementsCheck) on Draw's own Prevent$ family:
// living_conundrum.txt's own real "if you would draw a card while your
// library has no cards in it, instead you lose the game" is scoped here to
// just the prevention half (IsPresent$ Card.YouOwn | PresentZone$ Library |
// PresentCompare$ EQ0) -- met when the library is empty, so the draw is
// prevented.
func TestDrawPreventedWhenIsPresentConditionMet(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a")
	p := g.Players()[0]
	g.NewCard(replacementEnchantmentDef(t, "Test Conditional Draw Lock",
		"Event$ Draw | ActiveZones$ Battlefield | ValidPlayer$ You | IsPresent$ Card.YouOwn | PresentZone$ Library | PresentCompare$ EQ0 | Prevent$ True | Description$ conditional lock."), p, engine.Battlefield)

	g.DrawCards(p, 1, engine.NewScriptedController())

	if g.Player(p).DrewFromEmptyLibrary {
		t.Error("DrewFromEmptyLibrary = true, want false -- the draw must have been prevented before the empty-library check ever ran")
	}
}

// TestDrawNotPreventedWhenIsPresentConditionNotMet is the same lock's own
// mirror: with a card still in the library, IsPresent$'s own PresentCompare$
// EQ0 is not met, so the draw proceeds normally.
func TestDrawNotPreventedWhenIsPresentConditionNotMet(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a")
	p := g.Players()[0]
	top := g.NewCard(nil, p, engine.Library)
	g.NewCard(replacementEnchantmentDef(t, "Test Conditional Draw Lock",
		"Event$ Draw | ActiveZones$ Battlefield | ValidPlayer$ You | IsPresent$ Card.YouOwn | PresentZone$ Library | PresentCompare$ EQ0 | Prevent$ True | Description$ conditional lock."), p, engine.Battlefield)

	g.DrawCards(p, 1, engine.NewScriptedController())

	if g.Card(top).Zone != engine.Hand {
		t.Errorf("card zone = %v, want Hand -- the library is not empty, so IsPresent$ must not be met and the draw must proceed", g.Card(top).Zone)
	}
}

// TestDrawNotPreventedByUnresolvedOptional proves obstinate_familiar.txt's
// own real Optional$ True | Prevent$ True shape skips the whole line rather
// than preventing unconditionally (PORT-8/GO-7): this port's own
// PlayerController has no "may" hook for it.
func TestDrawNotPreventedByUnresolvedOptional(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a")
	p := g.Players()[0]
	top := g.NewCard(nil, p, engine.Library)
	g.NewCard(replacementEnchantmentDef(t, "Test Optional Draw Lock",
		"Event$ Draw | ActiveZones$ Battlefield | ValidPlayer$ You | Optional$ True | Prevent$ True | Description$ optional lock."), p, engine.Battlefield)

	g.DrawCards(p, 1, engine.NewScriptedController())

	if g.Card(top).Zone != engine.Hand {
		t.Errorf("card zone = %v, want Hand -- Optional$ is unresolved and must skip the whole line", g.Card(top).Zone)
	}
}

// TestGainLifePreventedByBareReplacement proves gainLifePrevented
// (replacement.go) is wired into gainLifeEffect.Resolve (gainlifeeffect.go):
// sulfuric_vortex.txt's own real "if a player would gain life, that player
// gains no life instead" (Prevent$ True, no ValidPlayer$ at all) stops the
// gain entirely.
func TestGainLifePreventedByBareReplacement(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p, other := g.Players()[0], g.Players()[1]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life, g.Player(other).Life = 20, 20
	g.NewCard(replacementEnchantmentDef(t, "Test No-Life Lock",
		"Event$ GainLife | ActiveZones$ Battlefield | Prevent$ True | Description$ No one gains life."), p, engine.Battlefield)

	if err := castETBGainLife(t, g, p, etbGainLifeTriggerDefParams(t, "Test GainLife", "Defined$ You | LifeAmount$ 5", nil)); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}

	if got := g.Player(p).Life; got != 20 {
		t.Errorf("p life = %d, want unchanged 20 -- Prevent$ True must stop the life gain entirely", got)
	}
}
