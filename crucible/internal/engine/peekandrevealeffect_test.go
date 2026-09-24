package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
)

// TestPeekAndRevealEffectRemembersTopCards proves the dominant shape (88 of
// 138 real lines): the top PeekAmount$ library cards are revealed and
// remembered, top first.
func TestPeekAndRevealEffectRemembersTopCards(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGame(t)
	lib := libraryCards(t, g, p, 3)

	c := engine.NewScriptedController()
	host, err := castETBChain(t, g, p, etbChainDef(t, "Test Peek", "DB$ PeekAndReveal | Defined$ You | PeekAmount$ 2 | RememberRevealed$ True"), c)
	if err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	want := []engine.EntityID{engine.CardEntity(lib[0]), engine.CardEntity(lib[1])}
	got := g.Card(host).Memory.Remembered()
	if len(got) != 2 || got[0] != want[0] || got[1] != want[1] {
		t.Errorf("Remembered = %v, want %v", got, want)
	}
}

// TestPeekAndRevealEffectDeclinedOptionalRevealRemembersNothing proves
// RevealOptional$: a declined reveal skips RememberRevealed$ entirely.
func TestPeekAndRevealEffectDeclinedOptionalRevealRemembersNothing(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGame(t)
	libraryCards(t, g, p, 2)

	c := engine.NewScriptedController()
	c.QueueConfirmReveal(false)
	host, err := castETBChain(t, g, p, etbChainDef(t, "Test Peek Optional", "DB$ PeekAndReveal | Defined$ You | RevealOptional$ True | RememberRevealed$ True"), c)
	if err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if got := g.Card(host).Memory.Remembered(); len(got) != 0 {
		t.Errorf("Remembered = %v, want empty", got)
	}
}

// TestPeekAndRevealEffectNoRevealRemembersPeeked proves NoReveal$ with
// RememberPeeked$: nothing is revealed, but the peeked card is still kept.
func TestPeekAndRevealEffectNoRevealRemembersPeeked(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGame(t)
	lib := libraryCards(t, g, p, 2)

	c := engine.NewScriptedController()
	host, err := castETBChain(t, g, p, etbChainDef(t, "Test Peek NoReveal", "DB$ PeekAndReveal | Defined$ You | NoReveal$ True | RememberPeeked$ True"), c)
	if err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if got := g.Card(host).Memory.Remembered(); len(got) != 1 || got[0] != engine.CardEntity(lib[0]) {
		t.Errorf("Remembered = %v, want [%v]", got, engine.CardEntity(lib[0]))
	}
}

// TestPeekAndRevealEffectImprintRevealed proves ImprintRevealed$ imprints
// the revealed card, readable as Defined$ Imprinted.
func TestPeekAndRevealEffectImprintRevealed(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGame(t)
	lib := libraryCards(t, g, p, 2)

	c := engine.NewScriptedController()
	host, err := castETBChain(t, g, p, etbChainDef(t, "Test Peek Imprint", "DB$ PeekAndReveal | ImprintRevealed$ True"), c)
	if err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if got := g.Card(host).Memory.Imprinted(); len(got) != 1 || got[0] != lib[0] {
		t.Errorf("Imprinted = %v, want [%v]", got, lib[0])
	}
}

// TestPeekAndRevealEffectRejectsUnresolvedParam proves SourceZone$ and a
// bad PeekAmount$ fail closed.
func TestPeekAndRevealEffectRejectsUnresolvedParam(t *testing.T) {
	t.Parallel()

	for _, line := range []string{
		"DB$ PeekAndReveal | SourceZone$ Graveyard",
		"DB$ PeekAndReveal | PeekAmount$ Bogus",
		"DB$ PeekAndReveal | Defined$ TriggeredPlayer",
	} {
		g, p, _ := newTwoPlayerGame(t)
		c := engine.NewScriptedController()
		if _, err := castETBChain(t, g, p, etbChainDef(t, "Test Peek Reject", line), c); err == nil {
			t.Errorf("%q: ResolveStack succeeded, want an error", line)
		}
	}
}
