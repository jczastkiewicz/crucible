package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
	"github.com/jczastkiewicz/crucible/internal/mana"
)

// TestCleanupEffectClearsMemoryAtChainEnd proves the corpus's dominant
// Cleanup shape: a chain that wrote Remembered/Chosen ends in DB$ Cleanup,
// and the host's Memory is empty afterwards -- the lists do not leak into
// the next time the card acts.
func TestCleanupEffectClearsMemoryAtChainEnd(t *testing.T) {
	t.Parallel()

	g, p, other := newTwoPlayerGame(t)
	pick := g.NewCard(creatureDefPT(t, "2", "2"), other, engine.Battlefield)

	c := engine.NewScriptedController()
	c.QueueCardChoice([]engine.CardID{pick})
	c.QueuePlayerChoice(other)
	def := etbChainDef(t, "Test Cleanup",
		"DB$ ChooseCard | Choices$ Creature.OppCtrl | Mandatory$ True | RememberChosen$ True | ImprintChosen$ True | SubAbility$ DBPlayer",
		"DBPlayer", "DB$ ChoosePlayer | Defined$ You | Choices$ Player.Opponent | SubAbility$ DBCleanup",
		"DBCleanup", "DB$ Cleanup | ClearRemembered$ True | ClearImprinted$ True | ClearChosenCard$ True | ClearChosenPlayer$ True")
	host, err := castETBChain(t, g, p, def, c)
	if err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	m := g.Card(host).Memory
	if len(m.Remembered()) != 0 || len(m.Imprinted()) != 0 || len(m.Chosen()) != 0 {
		t.Errorf("Memory after Cleanup: remembered %v, imprinted %v, chosen %v, want all empty",
			m.Remembered(), m.Imprinted(), m.Chosen())
	}
	if got := m.ChosenPlayer(); got != engine.NoPlayer {
		t.Errorf("ChosenPlayer after Cleanup = %v, want NoPlayer", got)
	}
}

// TestCleanupEffectLeavesUnnamedListsAlone proves each Clear*$ key is
// independent: ClearRemembered$ alone keeps the chosen card.
func TestCleanupEffectLeavesUnnamedListsAlone(t *testing.T) {
	t.Parallel()

	g, p, other := newTwoPlayerGame(t)
	pick := g.NewCard(creatureDefPT(t, "2", "2"), other, engine.Battlefield)

	c := engine.NewScriptedController()
	c.QueueCardChoice([]engine.CardID{pick})
	def := etbChainDef(t, "Test Cleanup Partial",
		"DB$ ChooseCard | Choices$ Creature.OppCtrl | Mandatory$ True | RememberChosen$ True | SubAbility$ DBCleanup",
		"DBCleanup", "DB$ Cleanup | ClearRemembered$ True")
	host, err := castETBChain(t, g, p, def, c)
	if err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	m := g.Card(host).Memory
	if len(m.Remembered()) != 0 {
		t.Errorf("Remembered = %v, want empty", m.Remembered())
	}
	if got := m.Chosen(); len(got) != 1 || got[0] != pick {
		t.Errorf("Chosen = %v, want [%v] untouched", got, pick)
	}
}

// TestCleanupEffectRejectsUnresolvedParam proves Log$ fails closed rather
// than being dropped (PORT-8).
func TestCleanupEffectRejectsUnresolvedParam(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGame(t)
	c := engine.NewScriptedController()
	def := etbChainDef(t, "Test Cleanup Log", "DB$ Cleanup | ClearRemembered$ True | Log$ Something")
	if _, err := castETBChain(t, g, p, def, c); err == nil {
		t.Fatal("ResolveStack succeeded, want an error for unresolved Log$")
	}
}

// TestCleanupEffectClearsChosenTypeAndNamedCard proves ClearChosenType$
// empties both chosen types and ClearNamedCard$ the named cards.
func TestCleanupEffectClearsChosenTypeAndNamedCard(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGame(t)
	c := engine.NewScriptedController()
	c.QueueOption(0)
	host := resolveLine(t, g, p, c, "DB$ ChooseType | Defined$ You | Type$ Card | SubAbility$ DBName",
		"DBName", "DB$ NameCard | Defined$ You | ChooseFromList$ Alpha | AtRandom$ True | SubAbility$ DBClean",
		"DBClean", "DB$ Cleanup | ClearChosenType$ True | ClearNamedCard$ True")
	m := g.Card(host).Memory
	if m.ChosenType(false) != "" || len(m.NamedCards()) != 0 {
		t.Errorf("chosen type %q, named %v; want both cleared", m.ChosenType(false), m.NamedCards())
	}
}

// TestCleanupEffectClearsChosenColor proves ClearChosenColor$ resets the
// value ChooseColor wrote.
func TestCleanupEffectClearsChosenColor(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGame(t)
	c := engine.NewScriptedController()
	c.QueueColorChoice(mana.Blue)
	def := etbChainDef(t, "Test Cleanup Color", "DB$ ChooseColor | Defined$ You | SubAbility$ DBCleanup",
		"DBCleanup", "DB$ Cleanup | ClearChosenColor$ True")
	host, err := castETBChain(t, g, p, def, c)
	if err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if got := g.Card(host).Memory.ChosenColors(); got != 0 {
		t.Errorf("ChosenColors = %v, want cleared", got)
	}
}
