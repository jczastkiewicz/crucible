package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
)

// TestRevealEffectRemembersChosenHandCard proves the dominant shape: the
// player picks a hand card matching RevealValid$, and RememberRevealed$
// hands it to the rest of the chain.
func TestRevealEffectRemembersChosenHandCard(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGame(t)
	pick := g.NewCard(creatureDefPT(t, "2", "2"), p, engine.Hand)
	g.NewCard(creatureDefPT(t, "2", "2"), p, engine.Hand)

	c := engine.NewScriptedController()
	c.QueueCardChoice([]engine.CardID{pick})
	host, err := castETBChain(t, g, p, etbChainDef(t, "Test Reveal", "DB$ Reveal | Defined$ You | RevealValid$ Creature | RememberRevealed$ True"), c)
	if err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if got := g.Card(host).Memory.Remembered(); len(got) != 1 || got[0] != engine.CardEntity(pick) {
		t.Errorf("Remembered = %v, want [%v]", got, engine.CardEntity(pick))
	}
}

// TestRevealEffectAnyNumberAllowsNone proves AnyNumber$ lowers the minimum
// to zero, so an empty reveal remembers nothing and is not an error.
func TestRevealEffectAnyNumberAllowsNone(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGame(t)
	g.NewCard(creatureDefPT(t, "2", "2"), p, engine.Hand)

	c := engine.NewScriptedController()
	c.QueueCardChoice(nil)
	host, err := castETBChain(t, g, p, etbChainDef(t, "Test Reveal Any", "DB$ Reveal | Defined$ You | AnyNumber$ True | RememberRevealed$ True"), c)
	if err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if got := g.Card(host).Memory.Remembered(); len(got) != 0 {
		t.Errorf("Remembered = %v, want empty", got)
	}
}

// TestRevealEffectRejectsCardOutsideHand proves a reveal answer that is not
// one of the offered hand cards is an error (GO-7).
func TestRevealEffectRejectsCardOutsideHand(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGame(t)
	g.NewCard(creatureDefPT(t, "2", "2"), p, engine.Hand)
	elsewhere := g.NewCard(creatureDefPT(t, "2", "2"), p, engine.Battlefield)

	c := engine.NewScriptedController()
	c.QueueCardChoice([]engine.CardID{elsewhere})
	def := etbChainDef(t, "Test Reveal Bad", "DB$ Reveal | Defined$ You | RememberRevealed$ True")
	if _, err := castETBChain(t, g, p, def, c); err == nil {
		t.Fatal("ResolveStack succeeded, want an error for a card not in hand")
	}
}

// TestRevealEffectRevealAllValid proves RevealAllValid$ reveals every
// matching hand card without asking.
func TestRevealEffectRevealAllValid(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGame(t)
	a := g.NewCard(creatureDefPT(t, "2", "2"), p, engine.Hand)
	b := g.NewCard(creatureDefPT(t, "2", "2"), p, engine.Hand)

	c := engine.NewScriptedController()
	host, err := castETBChain(t, g, p, etbChainDef(t, "Test Reveal All", "DB$ Reveal | Defined$ You | RevealAllValid$ Creature | RememberRevealed$ True"), c)
	if err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if got := g.Card(host).Memory.Remembered(); len(got) != 2 || got[0] != engine.CardEntity(a) || got[1] != engine.CardEntity(b) {
		t.Errorf("Remembered = %v, want [%v %v]", got, engine.CardEntity(a), engine.CardEntity(b))
	}
}

// TestRevealEffectRevealDefinedAndEmptyHand proves RevealDefined$ reveals
// the named cards, and a player with an empty hand is skipped.
func TestRevealEffectRevealDefinedAndEmptyHand(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGame(t)
	g.NewCard(creatureDefPT(t, "2", "2"), p, engine.Hand)
	c := engine.NewScriptedController()
	host, err := castETBChain(t, g, p, etbChainDef(t, "Test Reveal Defined", "DB$ Reveal | Defined$ Player | RevealDefined$ Self | RememberRevealed$ True"), c)
	if err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if got := g.Card(host).Memory.Remembered(); len(got) != 1 || got[0] != engine.CardEntity(host) {
		t.Errorf("Remembered = %v, want [%v] once (the empty-hand opponent skipped)", got, engine.CardEntity(host))
	}
}

// TestRevealEffectNumCardsOptional proves NumCards$ sets the count and
// Optional$ lets the chooser reveal fewer.
func TestRevealEffectNumCardsOptional(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGame(t)
	a := g.NewCard(creatureDefPT(t, "2", "2"), p, engine.Hand)
	g.NewCard(creatureDefPT(t, "2", "2"), p, engine.Hand)
	g.NewCard(creatureDefPT(t, "2", "2"), p, engine.Hand)

	c := engine.NewScriptedController()
	c.QueueCardChoice([]engine.CardID{a})
	host, err := castETBChain(t, g, p, etbChainDef(t, "Test Reveal Num", "DB$ Reveal | Defined$ You | NumCards$ 2 | Optional$ True | RememberRevealed$ True"), c)
	if err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if got := g.Card(host).Memory.Remembered(); len(got) != 1 {
		t.Errorf("Remembered = %v, want one card", got)
	}
}
