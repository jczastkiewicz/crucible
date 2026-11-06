package engine_test

import (
	"strings"
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
)

// dakkonGame is a corpus game in p's first main phase with Dakkon Blackblade
// Avatar seated for p and the named card in p's hand, the avatar's grant
// applied.
func dakkonGame(t *testing.T, inHand string) (*engine.Game, engine.PlayerID, engine.CardID) {
	t.Helper()
	g, p, _ := newTwoPlayerGameOn(t, scenarioDB(t))
	g.SetTurnState(1, p, engine.Main1)
	g.SeatAvatar(p, corpusCard(t, "Dakkon Blackblade Avatar"))
	card := g.NewCard(corpusCard(t, inHand), p, engine.Hand)
	engine.CheckStateBasedActions(g, engine.NewScriptedController())
	return g, p, card
}

// Dakkon's grant plays a colored card from hand as a copy of a basic land
// of one of its colors (PlayLandVariantEffect.java:25-65): Grizzly Bears
// becomes a Forest or Snow-Covered Forest on the battlefield, using the land
// drop.
func TestDakkonPlaysAColoredCardAsABasicLand(t *testing.T) {
	t.Parallel()

	g, p, bears := dakkonGame(t, "Grizzly Bears")
	c0 := engine.NewScriptedController()
	if !g.ActivateAbility(p, bears, 0, c0) {
		t.Fatal("the granted PlayLandVariant did not activate")
	}
	// An ST$ ability resolves at once, never on the stack (MagicStack.java:
	// 289-293).
	if n := g.StackLen(); n != 0 {
		t.Errorf("stack = %d after activation, want 0", n)
	}
	if err := g.TakePendingError(); err != nil {
		t.Fatal(err)
	}
	c := g.Card(bears)
	if c.Zone != engine.Battlefield {
		t.Fatalf("Grizzly Bears in %v, want the battlefield", c.Zone)
	}
	if name := c.Def.Name; name != "Forest" && name != "Snow-Covered Forest" {
		t.Errorf("played as %q, want a Forest", name)
	}
	if got := g.Player(p).LandsPlayed; got != 1 {
		t.Errorf("lands played = %d, want 1", got)
	}
}

// With the land drop used, no candidate can be played (Player.canPlayLand):
// nothing happens.
func TestDakkonWithNoLandDropLeftDoesNothing(t *testing.T) {
	t.Parallel()

	g, p, bears := dakkonGame(t, "Grizzly Bears")
	g.Player(p).LandsPlayed = 1
	g.ActivateAbility(p, bears, 0, engine.NewScriptedController())
	if got := g.Card(bears).Zone; got != engine.Hand {
		t.Errorf("Grizzly Bears in %v, want the hand", got)
	}
}

// Dakkon grants nothing to a colorless card (Affected$ Card.nonColorless);
// a Clone$ other than BasicLand or a Defined$ is refused.
func TestPlayLandVariantColorlessCardAndRefusedShapes(t *testing.T) {
	t.Parallel()

	g, p, thopter := dakkonGame(t, "Ornithopter")
	if g.ActivateAbility(p, thopter, 0, engine.NewScriptedController()) {
		t.Error("a colorless card was played through Dakkon's grant")
	}
	for _, line := range []string{"DB$ PlayLandVariant | Clone$ Land", "DB$ PlayLandVariant | Clone$ BasicLand | Defined$ Self"} {
		def := etbChainDef(t, "Test Shape", line)
		if _, err := castETBChain(t, g, p, def, engine.NewScriptedController()); err == nil || !strings.Contains(err.Error(), "PlayLandVariant") {
			t.Errorf("%q: err = %v, want a refusal", line, err)
		}
	}
}
