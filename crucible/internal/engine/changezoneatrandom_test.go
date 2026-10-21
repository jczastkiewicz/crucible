package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
)

// TestChangeZoneAtRandomPicksWithoutAsking proves AtRandom$ on a hidden
// ChangeZone (ChangeZoneEffect.java:1195-1199): ChangeNum$ cards leave the
// searched zone with no controller prompt, the same seed picks the same
// cards, and a list of one card draws nothing.
func TestChangeZoneAtRandomPicksWithoutAsking(t *testing.T) {
	t.Parallel()

	run := func(graves, num int) (moved []engine.CardID, left int) {
		g, p, _ := newTwoPlayerGame(t)
		libraryCards(t, g, p, 1)
		for i := 0; i < graves; i++ {
			g.NewCard(creatureDefPT(t, "1", "1"), p, engine.Graveyard)
		}
		line := "DB$ ChangeZone | Hidden$ True | Origin$ Graveyard | Destination$ Exile | ChangeType$ Card | Mandatory$ True | AtRandom$ True"
		if num > 1 {
			line += " | ChangeNum$ 2"
		}
		def := etbChainDef(t, "Test Random Return", line)
		if _, err := castETBChain(t, g, p, def, engine.NewScriptedController()); err != nil {
			t.Fatalf("ResolveStack: %v", err)
		}
		return g.Zone(engine.Exile, p).Cards(), len(g.Zone(engine.Graveyard, p).Cards())
	}

	moved, left := run(3, 2)
	if len(moved) != 2 || left != 1 {
		t.Fatalf("exiled %d, left %d, want 2 and 1", len(moved), left)
	}
	again, _ := run(3, 2)
	if len(again) != 2 || again[0] != moved[0] || again[1] != moved[1] {
		t.Errorf("same seed picked %v then %v", moved, again)
	}
	if one, left := run(1, 1); len(one) != 1 || left != 0 {
		t.Errorf("single card: exiled %d, left %d, want 1 and 0", len(one), left)
	}
}

// TestChangeZoneWithoutDestinationOnlyChooses proves a hidden-origin
// ChangeZone with no Destination$ moves nothing but still applies
// RememberChanged$ (ChangeZoneEffect.java:1311-1313, :1476-1477): Turtles
// Forever's first half.
func TestChangeZoneWithoutDestinationOnlyChooses(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGame(t)
	libraryCards(t, g, p, 1)
	held := g.NewCard(creatureDefPT(t, "1", "1"), p, engine.Graveyard)
	c := engine.NewScriptedController()
	c.QueueCardChoice([]engine.CardID{held})
	def := etbChainDef(t, "Test Choose Only",
		"DB$ ChangeZone | Hidden$ True | Origin$ Graveyard | ChangeType$ Card | Mandatory$ True | RememberChanged$ True")
	host, err := castETBChain(t, g, p, def, c)
	if err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if got := g.Card(held).Zone; got != engine.Graveyard {
		t.Errorf("chosen card moved to %v, want it to stay in the graveyard", got)
	}
	if got := g.Card(host).Memory.Remembered(); len(got) != 1 || got[0] != engine.CardEntity(held) {
		t.Errorf("remembered %v, want the chosen card", got)
	}
}
