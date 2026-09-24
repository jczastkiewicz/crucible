package engine_test

import (
	"strings"
	"testing"

	"github.com/jczastkiewicz/crucible/internal/carddb"
	"github.com/jczastkiewicz/crucible/internal/carddb/compile"
	"github.com/jczastkiewicz/crucible/internal/cardtype"
	"github.com/jczastkiewicz/crucible/internal/engine"
	"github.com/jczastkiewicz/crucible/internal/mana"
)

// etbManaTriggerDefParams builds a *compile.Card whose own "when CARDNAME
// enters" trigger runs DB$ Mana with the given params string appended --
// etbDestroyTriggerDefParams' own shape (destroyeffect_test.go).
func etbManaTriggerDefParams(t *testing.T, name, extraParams string) *compile.Card {
	t.Helper()

	reg, err := cardtype.LoadRegistry(strings.NewReader("[CreatureTypes]\nElf\n"))
	if err != nil {
		t.Fatalf("LoadRegistry: %v", err)
	}
	raw := &carddb.Card{Filename: name}
	raw.Faces[0].Present = true
	raw.Faces[0].Name = name
	raw.Faces[0].Type = cardtype.Parse(reg, "Creature Elf")
	raw.Faces[0].Power, raw.Faces[0].Toughness = "1", "1"
	raw.Faces[0].ManaCost = mana.MustParse("G")
	raw.Faces[0].Triggers = []string{
		"Mode$ ChangesZone | Origin$ Any | Destination$ Battlefield | ValidCard$ Card.Self | Execute$ TrigMana",
	}
	raw.Faces[0].SVars.Set("TrigMana", "DB$ Mana | "+extraParams)

	c, err := compile.Compile(raw)
	if err != nil {
		t.Fatalf("compile %q: %v", name, err)
	}
	return c
}

// castETBMana casts def for p on controller c and resolves the stack --
// castETBDestroy's own shape (destroyeffect_test.go).
func castETBMana(t *testing.T, g *engine.Game, p engine.PlayerID, def *compile.Card, c *engine.ScriptedController) (engine.CardID, error) {
	t.Helper()
	g.Player(p).ManaPool.Add(mana.Green, 1)
	creature := g.NewCard(def, p, engine.Hand)
	if !g.CastSpell(p, creature, c) {
		t.Fatal("CastSpell failed casting a creature with exactly enough mana")
	}
	return creature, g.ResolveStack(engine.NewRegistry(), c)
}

// TestManaEffectLiteralColorAddsToPool proves the corpus's own dominant real
// shape: a single literal Produced$ symbol adds Amount$ of that color.
func TestManaEffectLiteralColorAddsToPool(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p := g.Players()[0]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life, g.Player(g.Players()[1]).Life = 20, 20

	c := engine.NewScriptedController()
	_, err := castETBMana(t, g, p, etbManaTriggerDefParams(t, "Test Mana Literal", "Produced$ R | Amount$ 2 | Defined$ You"), c)
	if err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	got := g.Player(p).ManaPool.Breakdown()
	if got[3] != 2 {
		t.Errorf("red = %d, want 2", got[3])
	}
}

// TestManaEffectColorlessAddsGeneric proves Produced$ C adds colorless mana,
// not a fifth color.
func TestManaEffectColorlessAddsGeneric(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p := g.Players()[0]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life, g.Player(g.Players()[1]).Life = 20, 20

	c := engine.NewScriptedController()
	_, err := castETBMana(t, g, p, etbManaTriggerDefParams(t, "Test Mana Colorless", "Produced$ C | Defined$ You"), c)
	if err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	got := g.Player(p).ManaPool.Breakdown()
	if got[5] != 1 {
		t.Errorf("colorless = %d, want 1", got[5])
	}
}

// TestManaEffectAnyAsksTheControllerToChoose proves Produced$ Any reuses
// ChooseManaColor (activatemanaability.go's own first caller, control.go)
// rather than resolving a fixed shape.
func TestManaEffectAnyAsksTheControllerToChoose(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p := g.Players()[0]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life, g.Player(g.Players()[1]).Life = 20, 20

	c := engine.NewScriptedController()
	c.QueueManaColor(mana.Blue)
	_, err := castETBMana(t, g, p, etbManaTriggerDefParams(t, "Test Mana Any", "Produced$ Any | Defined$ You"), c)
	if err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	got := g.Player(p).ManaPool.Breakdown()
	if got[1] != 1 {
		t.Errorf("blue = %d, want 1", got[1])
	}
}

// TestManaEffectComboOffersOnlyTheNamedColors proves Produced$ Combo <letters>
// restricts ChooseManaColor's own offered set -- parseComboColors'
// (activatemanaability.go) own real shape.
func TestManaEffectComboOffersOnlyTheNamedColors(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p := g.Players()[0]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life, g.Player(g.Players()[1]).Life = 20, 20

	c := engine.NewScriptedController()
	c.QueueManaColor(mana.Green)
	_, err := castETBMana(t, g, p, etbManaTriggerDefParams(t, "Test Mana Combo", "Produced$ Combo R G | Defined$ You"), c)
	if err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	got := g.Player(p).ManaPool.Breakdown()
	if got[4] != 1 {
		t.Errorf("green = %d, want 1", got[4])
	}
}

// TestManaEffectRejectsUnresolvedParam proves manaUnresolvedParams' own
// fail-loud contract (PORT-8/GO-7).
func TestManaEffectRejectsUnresolvedParam(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p := g.Players()[0]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life, g.Player(g.Players()[1]).Life = 20, 20

	c := engine.NewScriptedController()
	_, err := castETBMana(t, g, p, etbManaTriggerDefParams(t, "Test Mana Restrict", "Produced$ G | Defined$ You | RestrictValid$ CumulativeUpkeep"), c)
	if err == nil {
		t.Fatal("ResolveStack succeeded, want an error for unresolved RestrictValid$")
	}
}
