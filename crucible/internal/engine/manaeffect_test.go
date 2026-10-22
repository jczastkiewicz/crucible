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
	_, err := castETBMana(t, g, p, etbManaTriggerDefParams(t, "Test Mana Restrict", "Produced$ G | Defined$ You | RestrictValid$ Spell.wasCastFromExile"), c)
	if err == nil {
		t.Fatal("ResolveStack succeeded, want an error for unresolved RestrictValid$")
	}
}

// TestManaEffectOptionalAsksTheActivator proves Optional$ (Soulbright
// Flamekin, ManaEffect.java:52): a decline adds no mana.
func TestManaEffectOptionalAsksTheActivator(t *testing.T) {
	t.Parallel()

	for _, accept := range []bool{false, true} {
		g := newGame(t, "a", "b")
		p := g.Players()[0]
		g.SetTurnState(1, p, engine.Main1)
		g.Player(p).Life, g.Player(g.Players()[1]).Life = 20, 20
		c := engine.NewScriptedController()
		c.QueueConfirmEffect(accept)
		if _, err := castETBMana(t, g, p, etbManaTriggerDefParams(t, "Test Mana Optional", "Produced$ R | Defined$ You | Optional$ True"), c); err != nil {
			t.Fatalf("accept=%v: %v", accept, err)
		}
		if got := g.Player(p).ManaPool.Total(); (got > 0) != accept {
			t.Errorf("accept=%v: pool total = %d", accept, got)
		}
	}
}

// TestManaEffectSeveralColorsAddsOneOfEach proves Produced$ "R G" (Burning-Tree
// Emissary) adds one mana per named color, Amount$ times over
// (GameActionUtil.generatedMana repeats the whole string).
func TestManaEffectSeveralColorsAddsOneOfEach(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p := g.Players()[0]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life, g.Player(g.Players()[1]).Life = 20, 20

	_, err := castETBMana(t, g, p, etbManaTriggerDefParams(t, "Test Mana Pair", "Produced$ R G | Amount$ 2 | Defined$ You"), engine.NewScriptedController())
	if err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	got := g.Player(p).ManaPool.Breakdown()
	if got[3] != 2 || got[4] != 2 {
		t.Errorf("breakdown = %v, want 2 red and 2 green", got)
	}
}

// TestManaEffectComboAnyAsksForEachMana proves Produced$ Combo Any (Cosmic
// Crucible's four mana): one color choice per mana from all five colors,
// so two different answers give two different colors.
func TestManaEffectComboAnyAsksForEachMana(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p := g.Players()[0]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life, g.Player(g.Players()[1]).Life = 20, 20

	c := engine.NewScriptedController()
	c.QueueManaColor(mana.Red)
	c.QueueManaColor(mana.Green)
	if _, err := castETBMana(t, g, p, etbManaTriggerDefParams(t, "Test Mana Combo Any", "Produced$ Combo Any | Amount$ 2 | Defined$ You"), c); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if got := g.Player(p).ManaPool.Breakdown(); got[3] != 1 || got[4] != 1 {
		t.Errorf("pool = %v, want one red and one green", got)
	}
}

// TestManaEffectChosenWithoutAChosenColorMakesNothing proves
// ManaEffect.java:179-188: Produced$ Chosen on a host with no chosen color
// produces no mana and no error.
func TestManaEffectChosenWithoutAChosenColorMakesNothing(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p := g.Players()[0]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life, g.Player(g.Players()[1]).Life = 20, 20

	if _, err := castETBMana(t, g, p, etbManaTriggerDefParams(t, "Test Mana Chosen", "Produced$ Chosen | Defined$ You"), engine.NewScriptedController()); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	for i, n := range g.Player(p).ManaPool.Breakdown() {
		if n != 0 {
			t.Errorf("breakdown[%d] = %d, want an empty pool", i, n)
		}
	}
}
