package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
	"github.com/jczastkiewicz/crucible/internal/valid"
)

func exiledByYou(g *engine.Game, card, you engine.CardID) bool {
	return engine.Matches(g, g.Card(card), valid.Parse("Card.ExiledByYou"), g.Card(you).Controller(), you)
}

// TestExiledByYouFollowsTheExiler proves Card.ExiledByYou reads the player who
// exiled the card: the activator by default, DefinedExiler$ when the Dig names
// one (SpellAbilityEffect.java:1113-1114), and nobody for a card nothing exiled.
func TestExiledByYouFollowsTheExiler(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name, line string
		byP        bool
	}{
		{"activator by default", "DB$ Dig | DigNum$ 1 | ChangeNum$ All | DestinationZone$ Exile", true},
		{"DefinedExiler names the opponent", "DB$ Dig | DigNum$ 1 | ChangeNum$ All | DestinationZone$ Exile | DefinedExiler$ Opponent", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			g, p, other := newTwoPlayerGame(t)
			ids := libraryCards(t, g, p, 3)
			resolveLine(t, g, p, engine.NewScriptedController(), tc.line)
			top := engine.NoCard
			for _, id := range ids {
				if g.Card(id).Zone == engine.Exile {
					top = id
				}
			}
			if top == engine.NoCard {
				t.Fatal("no dug card reached Exile")
			}
			mine := g.NewCard(creatureDefPT(t, "1", "1"), p, engine.Battlefield)
			theirs := g.NewCard(creatureDefPT(t, "1", "1"), other, engine.Battlefield)
			if got := exiledByYou(g, top, mine); got != tc.byP {
				t.Errorf("exiled by the activator = %v, want %v", got, tc.byP)
			}
			if got := exiledByYou(g, top, theirs); got == tc.byP {
				t.Errorf("exiled by the opponent = %v, want %v", got, !tc.byP)
			}
		})
	}
	g, p, _ := newTwoPlayerGame(t)
	plain := g.NewCard(creatureDefPT(t, "1", "1"), p, engine.Exile)
	mine := g.NewCard(creatureDefPT(t, "1", "1"), p, engine.Battlefield)
	if exiledByYou(g, plain, mine) {
		t.Error("a card nothing exiled reads as exiled by you")
	}
}

// TestMayPlayPlayerExilerGrantsTheExiler proves MayPlayPlayer$ Exiler hands
// each affected card to the player who exiled it (StaticAbilityContinuous
// .java:942-944): the Dig names the opponent as exiler, so the static's
// controller may not cast the card but the opponent may.
func TestMayPlayPlayerExilerGrantsTheExiler(t *testing.T) {
	t.Parallel()

	g, p, other := newTwoPlayerGame(t)
	g.NewCard(continuousDef(t, "Shared",
		"Mode$ Continuous | MayPlayPlayer$ Exiler | Affected$ Card.YouOwn | AffectedZone$ Exile | MayPlay$ True | MayPlayWithoutManaCost$ True | MayPlayWithFlash$ True"),
		p, engine.Battlefield)
	card := g.NewCard(creatureDefCost(t, "Pricey", "7 G"), p, engine.Library)
	resolveLine(t, g, p, engine.NewScriptedController(),
		"DB$ Dig | DigNum$ 1 | ChangeNum$ All | DestinationZone$ Exile | DefinedExiler$ Opponent")
	sba(g)
	if g.Card(card).Zone != engine.Exile {
		t.Fatalf("dug card in %v, want Exile", g.Card(card).Zone)
	}
	if g.CastSpell(p, card, engine.NewScriptedController()) {
		t.Error("the owner cast a card the opponent exiled")
	}
	if !g.CastSpell(other, card, engine.NewScriptedController()) {
		t.Error("the exiler could not cast the card")
	}
}
