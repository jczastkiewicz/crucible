package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
)

// TestTurnFaceUpCantHappenHoldsOnlyDuringTheHostControllersTurn proves Karlov
// Watchdog's "permanents your opponents control can't be turned face up during
// your turn" (Event$ TurnFaceUp, Layer$ CantHappen, PlayerTurn$ True) stops a
// SetState Mode$ TurnFaceUp on the opponent's face-down permanent on the
// Watchdog controller's turn, and not on the opponent's own.
func TestTurnFaceUpCantHappenHoldsOnlyDuringTheHostControllersTurn(t *testing.T) {
	t.Parallel()

	g, p, other := newTwoPlayerGame(t)
	g.NewCard(replacementEnchantmentDef(t, "Test Watchdog",
		"Event$ TurnFaceUp | ValidCard$ Permanent.OppCtrl | Layer$ CantHappen | ActiveZones$ Battlefield | PlayerTurn$ True | Description$ No."),
		other, engine.Battlefield)
	top := g.NewCard(creatureDefPT(t, "5", "5"), p, engine.Library)
	resolveLine(t, g, p, engine.NewScriptedController(), "DB$ Manifest")
	if !g.Card(top).IsFaceDown() {
		t.Fatal("setup: the manifested card is not face down")
	}
	def := etbChainDef(t, "Turner", "DB$ SetState | ValidTgts$ Creature | Mode$ TurnFaceUp")
	host := g.NewCard(def, p, engine.Battlefield)
	sub := def.Faces[0].Triggers[0].Subs[0].Ability
	turnUp := func() {
		g.PushAbility(engine.Ability{API: engine.APISetState, Source: host, Controller: p, Params: sub,
			Targets: []engine.EntityID{engine.CardEntity(top)}})
		if err := g.ResolveStack(engine.NewRegistry(), engine.NewScriptedController()); err != nil {
			t.Fatal(err)
		}
	}

	g.SetTurnState(2, other, engine.Main1)
	turnUp()
	if !g.Card(top).IsFaceDown() {
		t.Error("turned face up on the Watchdog controller's own turn")
	}

	g.SetTurnState(3, p, engine.Main1)
	turnUp()
	if g.Card(top).IsFaceDown() {
		t.Error("still face down on the other player's turn")
	}
}

// TestMovedDayTimeReplacementMakesItDayOnlyWhenNeither proves the Event$ Moved
// "if it's neither day nor night, it becomes day as CARDNAME enters" shape
// (DayTime$ Neither, ReplaceWith$ DB$ DayTime | Value$ Day: Sunrise Cavalier's
// family, 10 real lines): entering sets day.
func TestMovedDayTimeReplacementMakesItDayOnlyWhenNeither(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a")
	p := g.Players()[0]
	g.SetTurnState(1, p, engine.Main1)
	line := "Event$ Moved | ValidCard$ Card.Self | Destination$ Battlefield | DayTime$ Neither | ReplaceWith$ DoDay | ReplacementResult$ Updated"
	land := g.NewCard(replacementHostDef(t, "Test Dawn Land", "Land", line, "DoDay", "DB$ DayTime | Value$ Day"), p, engine.Hand)

	if g.DayTime() != engine.DayNeither {
		t.Fatal("setup: not neither day nor night")
	}
	if !g.PlayLand(p, land, engine.NewScriptedController()) {
		t.Fatal("PlayLand failed")
	}
	if g.DayTime() != engine.Day {
		t.Errorf("daytime = %v, want Day after the replacement ran", g.DayTime())
	}
}
