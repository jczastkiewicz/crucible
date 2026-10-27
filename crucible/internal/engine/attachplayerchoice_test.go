package engine_test

import (
	"strings"
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
)

// Lynde, Cheerful Tormentor: "you may attach a Curse attached to you to one
// of your opponents" (AttachEffect.java:81-152, Object$ with PlayerChoices$).
const lyndeChain = "DB$ ChooseCard | Choices$ Curse.AttachedTo You | SubAbility$ DBAttach"

func TestAttachObjectPlayerChoicesMovesTheCurseToTheOpponent(t *testing.T) {
	t.Parallel()

	g, p, other := newTwoPlayerGame(t)
	curse := g.NewCard(scriptDef(t, "Test Curse", "Enchantment Aura Curse", "K:Enchant:Player"), p, engine.Battlefield)
	g.AttachToPlayer(curse, p)
	c := engine.NewScriptedController()
	c.QueueCardChoice([]engine.CardID{curse})
	def := etbChainDef(t, "Test Lynde", lyndeChain,
		"DBAttach", "DB$ Attach | Object$ ChosenCard | PlayerChoices$ Opponent | RememberAttached$ True")
	if _, err := castETBChain(t, g, p, def, c); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if host, ok := g.Card(curse).AttachedToPlayer(); !ok || host != other {
		t.Errorf("curse attached to %v (%v), want the opponent %v", host, ok, other)
	}
}

// An Aura that cannot enchant an opponent makes Java throw
// ConcurrentModificationException while pruning the choices
// (AttachEffect.java:83-88): refused by name, nothing moves.
func TestAttachObjectPlayerChoicesRefusesAPlayerTheCurseCannotEnchant(t *testing.T) {
	t.Parallel()

	g, p, other := newTwoPlayerGame(t)
	curse := g.NewCard(scriptDef(t, "Test Opponent Curse", "Enchantment Aura Curse", "K:Enchant:Opponent"), p, engine.Battlefield)
	g.AttachToPlayer(curse, other)
	c := engine.NewScriptedController()
	c.QueueCardChoice([]engine.CardID{curse})
	def := etbChainDef(t, "Test Lynde Refused", "DB$ ChooseCard | Choices$ Curse | SubAbility$ DBAttach",
		"DBAttach", "DB$ Attach | Object$ ChosenCard | PlayerChoices$ You")
	_, err := castETBChain(t, g, p, def, c)
	if err == nil || !strings.Contains(err.Error(), "ConcurrentModificationException") {
		t.Fatalf("err = %v, want one naming the Java ConcurrentModificationException", err)
	}
	if host, _ := g.Card(curse).AttachedToPlayer(); host != other {
		t.Errorf("curse moved to %v, want it left on %v", host, other)
	}
}
