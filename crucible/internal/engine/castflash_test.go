package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
	"github.com/jczastkiewicz/crucible/internal/mana"
)

// castOnOpponentsTurn tries to cast card for p with {G} on the other player's
// main phase, where only an instant, or a spell cast as though it had flash,
// is legal (CR 307.1).
func castOnOpponentsTurn(t *testing.T, g *engine.Game, p, other engine.PlayerID, card engine.CardID) bool {
	t.Helper()
	g.SetTurnState(1, other, engine.Main1)
	g.Player(p).ManaPool.Add(mana.Green, 1)
	return g.CastSpell(p, card, engine.NewScriptedController())
}

// CR 702.36a: a creature spell with flash may be cast any time its controller
// has priority; the same creature without it needs sorcery timing.
func TestFlashKeywordLetsAPermanentSpellBeCastAtInstantSpeed(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name     string
		keywords []string
		want     bool
	}{
		{"with flash", []string{"Flash"}, true},
		{"without flash", nil, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			g, p, other := newTwoPlayerGame(t)
			def := creatureDefCost(t, "Test Flash", "G")
			def.Faces[0].Keywords = tc.keywords
			card := g.NewCard(def, p, engine.Hand)
			if got := castOnOpponentsTurn(t, g, p, other, card); got != tc.want {
				t.Errorf("CastSpell on the opponent's turn = %v, want %v", got, tc.want)
			}
		})
	}
}

// StaticAbilityCastWithFlash: "you may cast creature spells as though they had
// flash" lets the controller's creature spells through, but not the
// opponent's, and not a noncreature one the ValidCard$ does not name.
func TestCastWithFlashStaticGrantsFlashToMatchingSpells(t *testing.T) {
	t.Parallel()

	g, p, other := newTwoPlayerGame(t)
	g.NewCard(scriptDef(t, "Test Vedalken", "Enchantment",
		"S:Mode$ CastWithFlash | ValidCard$ Creature | ValidSA$ Spell | Caster$ You"), p, engine.Battlefield)
	mine := g.NewCard(creatureDefCost(t, "Mine", "G"), p, engine.Hand)
	theirs := g.NewCard(creatureDefCost(t, "Theirs", "G"), other, engine.Hand)

	if !castOnOpponentsTurn(t, g, p, other, mine) {
		t.Error("the static's controller cannot cast their creature at instant speed")
	}
	g.SetTurnState(1, p, engine.Main1)
	g.Player(other).ManaPool.Add(mana.Green, 1)
	if g.CastSpell(other, theirs, engine.NewScriptedController()) {
		t.Error("the opponent cast a creature on the other player's turn; Caster$ You names only the static's controller")
	}
}

// A CastWithFlash line this port cannot evaluate (a ValidSA$ past plain
// Spell, an IsPresent$ condition) grants nothing rather than guessing.
func TestCastWithFlashStaticSkipsShapesItCannotEvaluate(t *testing.T) {
	t.Parallel()

	g, p, other := newTwoPlayerGame(t)
	g.NewCard(scriptDef(t, "Test Conditional", "Enchantment",
		"S:Mode$ CastWithFlash | ValidCard$ Creature | ValidSA$ Spell | Caster$ You | IsPresent$ Card.Self+powerOdd"), p, engine.Battlefield)
	card := g.NewCard(creatureDefCost(t, "Mine", "G"), p, engine.Hand)

	if castOnOpponentsTurn(t, g, p, other, card) {
		t.Error("a conditional CastWithFlash line granted flash")
	}
}
