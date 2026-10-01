package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
	"github.com/jczastkiewicz/crucible/internal/mana"
)

// cantBeCastGame is p's turn (Main1) with a "Test Rule" enchantment carrying
// line under p's control, and a {G} creature in each player's hand.
func cantBeCastGame(t *testing.T, line string) (g *engine.Game, p, other engine.PlayerID, mine, theirs engine.CardID) {
	t.Helper()
	g, p, other = newTwoPlayerGame(t)
	g.NewCard(scriptDef(t, "Test Rule", "Enchantment", line), p, engine.Battlefield)
	mine = g.NewCard(creatureDefCost(t, "Mine", "G"), p, engine.Hand)
	theirs = g.NewCard(creatureDefCost(t, "Theirs", "G"), other, engine.Hand)
	return g, p, other, mine, theirs
}

func castWithG(g *engine.Game, pid engine.PlayerID, card engine.CardID) bool {
	g.Player(pid).ManaPool.Add(mana.Green, 1)
	return g.CastSpell(pid, card, engine.NewScriptedController())
}

// Mode$ CantBeCast with Caster$ and ValidCard$ stops only the named casters
// from casting the named cards.
func TestCantBeCastNamesCasterAndCard(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name             string
		line             string
		mineOK, theirsOK bool
	}{
		{"opponents cannot cast anything", "S:Mode$ CantBeCast | ValidCard$ Card | Caster$ Opponent", true, false},
		{"you cannot cast creatures", "S:Mode$ CantBeCast | ValidCard$ Creature | Caster$ You", false, true},
		{"nobody casts noncreature spells", "S:Mode$ CantBeCast | ValidCard$ Card.nonCreature | Caster$ Player", true, true},
		{"nobody casts creatures", "S:Mode$ CantBeCast | ValidCard$ Creature", false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			g, p, other, mine, theirs := cantBeCastGame(t, tc.line)
			if got := castWithG(g, p, mine); got != tc.mineOK {
				t.Errorf("the static controller's cast = %v, want %v", got, tc.mineOK)
			}
			// the opponent casts on its own turn, from an empty stack, so timing
			// never refuses it
			if err := g.ResolveStack(engine.NewRegistry(), engine.NewScriptedController()); err != nil {
				t.Fatalf("ResolveStack: %v", err)
			}
			g.SetTurnState(2, other, engine.Main1)
			if got := castWithG(g, other, theirs); got != tc.theirsOK {
				t.Errorf("the opponent's cast = %v, want %v", got, tc.theirsOK)
			}
		})
	}
}

// Condition$ PlayerTurn: "your opponents can't cast spells during your turn".
func TestCantBeCastHonoursItsCondition(t *testing.T) {
	t.Parallel()

	g, _, other, _, theirs := cantBeCastGame(t,
		"S:Mode$ CantBeCast | ValidCard$ Card | Condition$ PlayerTurn | Caster$ Opponent")
	// a flash creature, so timing alone would let the opponent cast on p's turn
	flash := creatureDefCost(t, "Flash", "G")
	flash.Faces[0].Keywords = []string{"Flash"}
	quick := g.NewCard(flash, other, engine.Hand)
	if castWithG(g, other, quick) || castWithG(g, other, theirs) {
		t.Error("the opponent cast during the static controller's turn")
	}
	g.SetTurnState(2, other, engine.Main1)
	if !castWithG(g, other, theirs) {
		t.Error("the opponent could not cast on their own turn, where the condition fails")
	}
}

// NumLimitEachTurn$ 1 (Rule of Law): a player's second spell in a turn is
// refused, the first is not.
func TestCantBeCastNumLimitEachTurn(t *testing.T) {
	t.Parallel()

	g, p, _, mine, _ := cantBeCastGame(t, "S:Mode$ CantBeCast | ValidCard$ Card | Caster$ Player | NumLimitEachTurn$ 1")
	second := g.NewCard(creatureDefCost(t, "Second", "G"), p, engine.Hand)
	if !castWithG(g, p, mine) {
		t.Fatal("the first spell of the turn was refused")
	}
	if castWithG(g, p, second) {
		t.Error("a second spell in the same turn was cast under NumLimitEachTurn$ 1")
	}
}

// Phases$: no spells from the beginning of combat to its end.
func TestCantBeCastPhases(t *testing.T) {
	t.Parallel()

	g, p, _, mine, _ := cantBeCastGame(t, "S:Mode$ CantBeCast | ValidCard$ Card | Phases$ BeginCombat->EndCombat")
	if !castWithG(g, p, mine) {
		t.Error("a main-phase cast was refused by a combat-only static")
	}
}

// A line this port cannot evaluate (cmcGT$) is not applied rather than guessed.
func TestCantBeCastSkipsUnresolvableLines(t *testing.T) {
	t.Parallel()

	g, p, _, mine, _ := cantBeCastGame(t, "S:Mode$ CantBeCast | ValidCard$ Card | Caster$ You | cmcGT$ Land")
	if !castWithG(g, p, mine) {
		t.Error("an unresolvable CantBeCast line refused the cast")
	}
}
