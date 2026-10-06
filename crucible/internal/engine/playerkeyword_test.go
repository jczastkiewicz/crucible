package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
)

// Player.HasKeyword reads the keyword a continuous static grants the player
// (Java's Player.hasKeyword): present while the granting permanent is, absent
// for any other keyword name and for a player nothing grants anything to.
func TestPlayerHasKeywordReadsGrantedKeywords(t *testing.T) {
	t.Parallel()

	g, p, opp := newTwoPlayerGameOn(t, scenarioDB(t))
	g.NewCard(scriptDef(t, "Test Aegis", "Artifact",
		"S:Mode$ Continuous | Affected$ You | AddKeyword$ Hexproof"), opp, engine.Battlefield)
	engine.CheckStateBasedActions(g, engine.NewScriptedController())

	if !g.Player(opp).HasKeyword("Hexproof") {
		t.Error("the granted player lacks Hexproof")
	}
	if g.Player(opp).HasKeyword("Shroud") {
		t.Error("the granted player has Shroud nothing granted")
	}
	if g.Player(p).HasKeyword("Hexproof") {
		t.Error("the other player has Hexproof, which only the controller's static grants")
	}
}

// DiscardSink swallows every event: a simulated game run on it records
// nothing.
func TestDiscardSinkSwallowsEvents(t *testing.T) {
	t.Parallel()

	engine.DiscardSink{}.Emit(engine.Event{})
}
