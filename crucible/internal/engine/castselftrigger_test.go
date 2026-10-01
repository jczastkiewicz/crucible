package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/carddb/compile"
	"github.com/jczastkiewicz/crucible/internal/engine"
	"github.com/jczastkiewicz/crucible/internal/mana"
)

// castSelfTriggerDef is a {G} creature carrying "When you cast this spell,
// you gain 3 life" with extra params on the trigger line (Abby, Merciless
// Soldier's shape: Mode$ SpellCast | ValidCard$ Card.Self, no TriggerZones$).
func castSelfTriggerDef(t *testing.T, extra string) *compile.Card {
	t.Helper()
	def := scriptDef(t, "Test Cast Self", "Creature Elf",
		"T:Mode$ SpellCast | ValidCard$ Card.Self"+extra+" | Execute$ TrigGain",
		"SVar:TrigGain:DB$ GainLife | Defined$ You | LifeAmount$ 3")
	def.Faces[0].ManaCost = mana.MustParse("G")
	return def
}

// castGreenAndResolve casts id for p with one green mana and resolves the stack.
func castGreenAndResolve(t *testing.T, g *engine.Game, p engine.PlayerID, id engine.CardID, c *engine.ScriptedController) {
	t.Helper()
	g.Player(p).ManaPool.Add(mana.Green, 1)
	if !g.CastSpell(p, id, c) {
		t.Fatal("CastSpell failed casting a {G} spell with one green mana")
	}
	if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
}

// A spell's own "when you cast this spell" trigger fires from the stack: Java
// collects every card's triggers in every zone and a trigger with no
// TriggerZones$ is active anywhere (TriggerReplacementBase.java:61-65).
func TestSpellsOwnCastTriggerFiresFromTheStack(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGame(t)
	spell := g.NewCard(castSelfTriggerDef(t, ""), p, engine.Hand)
	castGreenAndResolve(t, g, p, spell, engine.NewScriptedController())

	if got := g.Player(p).Life; got != 23 {
		t.Errorf("life = %d, want 23 -- the spell's own SpellCast trigger should have resolved", got)
	}
	if got := g.Card(spell).Zone; got != engine.Battlefield {
		t.Errorf("spell zone = %v, want Battlefield", got)
	}
}

// A cast trigger whose Execute$ carries its own Cost$ (Bearer of Silence's
// "you may pay {1}{C}") fires from the stack and asks: declined, the cost is not
// paid and the effect does not happen; confirmed and paid, it does.
func TestSpellsOwnCastTriggerWithExecuteCostPaysIt(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name     string
		confirm  bool
		wantLife int
	}{
		{"declined", false, 20},
		{"paid", true, 23},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			g, p, _ := newTwoPlayerGame(t)
			def := scriptDef(t, "Test Cast Cost", "Creature Elf",
				"T:Mode$ SpellCast | ValidCard$ Card.Self | Execute$ TrigGain",
				"SVar:TrigGain:AB$ GainLife | Cost$ G | Defined$ You | LifeAmount$ 3")
			def.Faces[0].ManaCost = mana.MustParse("G")
			spell := g.NewCard(def, p, engine.Hand)
			c := engine.NewScriptedController()
			c.QueueConfirmOptionalTrigger(tc.confirm)
			g.Player(p).ManaPool.Add(mana.Green, 1) // pays the trigger's {G}; castGreenAndResolve adds the spell's
			castGreenAndResolve(t, g, p, spell, c)

			if got := g.Player(p).Life; got != tc.wantLife {
				t.Errorf("life = %d, want %d", got, tc.wantLife)
			}
			if got := g.Player(p).ManaPool.Total(); got != 1-boolToInt(tc.confirm) {
				t.Errorf("mana left = %d, want %d: the trigger's cost is paid only when confirmed", got, 1-boolToInt(tc.confirm))
			}
		})
	}
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// TriggerZones$ naming the Stack keeps it active there; naming only the
// battlefield does not, so the same trigger stays silent while cast.
func TestSpellsOwnCastTriggerHonoursTriggerZones(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		zones string
		want  int
	}{
		{" | TriggerZones$ Stack", 23},
		{" | TriggerZones$ Battlefield", 20},
	} {
		g, p, _ := newTwoPlayerGame(t)
		spell := g.NewCard(castSelfTriggerDef(t, tt.zones), p, engine.Hand)
		castGreenAndResolve(t, g, p, spell, engine.NewScriptedController())
		if got := g.Player(p).Life; got != tt.want {
			t.Errorf("%s: life = %d, want %d", tt.zones, got, tt.want)
		}
	}
}
