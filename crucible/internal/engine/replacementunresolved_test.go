package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
)

// TestDrawCardsReplacementUnresolvableShapesRecordErrors proves
// playerAmountReplaced skips a line naming a param it does not read, and a
// ReplaceEffect naming another event's number, recording a pending error
// instead of guessing (GO-7).
func TestDrawCardsReplacementUnresolvableShapesRecordErrors(t *testing.T) {
	t.Parallel()

	for name, tc := range map[string]struct{ line, sub string }{
		"param":  {"Event$ DrawCards | ActiveZones$ Battlefield | Weird$ True | ReplaceWith$ More", "DB$ Draw | Defined$ You"},
		"effect": {"Event$ DrawCards | ActiveZones$ Battlefield | ReplaceWith$ More", "DB$ ReplaceEffect | VarName$ Bogus | VarValue$ 1"},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			g := newGame(t, "a")
			p := g.Players()[0]
			g.NewCard(nil, p, engine.Library)
			g.NewCard(replacementEnchantmentDefWithSVar(t, "Test Odd", tc.line, "More", tc.sub), p, engine.Battlefield)
			g.DrawCards(p, 1, engine.NewScriptedController())
			if g.TakePendingError() == nil {
				t.Error("no pending error recorded")
			}
		})
	}
}

// TestTurnFaceUpUnresolvableLinesRecordErrors proves canBeTurnedFaceUp and
// faceChangeReplaced record a pending error for a line naming a param they do
// not read, without blocking the flip.
func TestTurnFaceUpUnresolvableLinesRecordErrors(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGame(t)
	g.NewCard(replacementEnchantmentDefWithSVar(t, "Test Odd Faces",
		"Event$ TurnFaceUp | ActiveZones$ Battlefield | Layer$ CantHappen | Weird$ True", "Rm", "DB$ Sacrifice"), p, engine.Battlefield)
	g.NewCard(replacementEnchantmentDefWithSVar(t, "Test Odd Faces Two",
		"Event$ TurnFaceUp | ActiveZones$ Battlefield | Weird$ True | ReplaceWith$ Rm", "Rm", "DB$ Sacrifice"), p, engine.Battlefield)
	top := g.NewCard(creatureDefPT(t, "5", "5"), p, engine.Library)
	resolveLine(t, g, p, engine.NewScriptedController(), "DB$ Manifest")
	def := etbChainDef(t, "Turner", "DB$ SetState | ValidTgts$ Creature | Mode$ TurnFaceUp")
	host := g.NewCard(def, p, engine.Battlefield)
	sub := def.Faces[0].Triggers[0].Subs[0].Ability
	g.PushAbility(engine.Ability{API: engine.APISetState, Source: host, Controller: p, Params: sub,
		Targets: []engine.EntityID{engine.CardEntity(top)}})
	if err := g.ResolveStack(engine.NewRegistry(), engine.NewScriptedController()); err == nil {
		t.Error("no pending error for the unreadable params")
	}
	if g.Card(top).IsFaceDown() {
		t.Error("the unreadable CantHappen line blocked the flip")
	}
}
