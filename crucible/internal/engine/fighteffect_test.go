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

// etbFightTriggerDefParams builds a *compile.Card whose own "when CARDNAME
// enters" trigger runs DB$ Fight with the given params string appended --
// etbDestroyTriggerDefParams' own shape (destroyeffect_test.go).
func etbFightTriggerDefParams(t *testing.T, name, extraParams string, svars map[string]string) *compile.Card {
	t.Helper()

	reg, err := cardtype.LoadRegistry(strings.NewReader("[CreatureTypes]\nElf\n"))
	if err != nil {
		t.Fatalf("LoadRegistry: %v", err)
	}
	raw := &carddb.Card{Filename: name}
	raw.Faces[0].Present = true
	raw.Faces[0].Name = name
	raw.Faces[0].Type = cardtype.Parse(reg, "Creature Elf")
	// Toughness 10, well past any test's own damage total, so a fought
	// creature survives the fight and stays on the battlefield with its
	// own marked damage intact -- ResolveStack's own CheckStateBasedActions
	// pass (stack.go) runs right after this ability resolves, and a lethally
	// damaged creature's own Move to the graveyard resets Damage.Marked to
	// 0 (CR 400.7), which would make a damage assertion read the wrong zone
	// change's own effect rather than this one's.
	raw.Faces[0].Power, raw.Faces[0].Toughness = "2", "10"
	raw.Faces[0].ManaCost = mana.MustParse("G")
	raw.Faces[0].Triggers = []string{
		"Mode$ ChangesZone | Origin$ Any | Destination$ Battlefield | ValidCard$ Card.Self | Execute$ TrigFight",
	}
	raw.Faces[0].SVars.Set("TrigFight", "DB$ Fight | "+extraParams)
	for name, body := range svars {
		raw.Faces[0].SVars.Set(name, body)
	}

	c, err := compile.Compile(raw)
	if err != nil {
		t.Fatalf("compile %q: %v", name, err)
	}
	return c
}

// castETBFight casts def for p on controller c and resolves the stack --
// castETBDestroy's own shape (destroyeffect_test.go).
func castETBFight(t *testing.T, g *engine.Game, p engine.PlayerID, def *compile.Card, c *engine.ScriptedController) (engine.CardID, error) {
	t.Helper()
	g.Player(p).ManaPool.Add(mana.Green, 1)
	creature := g.NewCard(def, p, engine.Hand)
	if !g.CastSpell(p, creature, c) {
		t.Fatal("CastSpell failed casting a creature with exactly enough mana")
	}
	return creature, g.ResolveStack(engine.NewRegistry(), c)
}

// TestFightEffectDefinedSelfFightsTarget proves the corpus's own dominant
// real shape (138 of 146 real lines): Defined$ Self combined with a chosen
// ValidTgts$ target -- fightFighters' own "fighter1 = the target, fighter2
// = the Defined$ card" combining rule.
func TestFightEffectDefinedSelfFightsTarget(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p := g.Players()[0]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life, g.Player(g.Players()[1]).Life = 20, 20
	target := g.NewCard(creatureDefPT(t, "3", "3"), p, engine.Battlefield)

	c := engine.NewScriptedController()
	c.QueueTargets([]engine.EntityID{engine.CardEntity(target)})

	host, err := castETBFight(t, g, p, etbFightTriggerDefParams(t, "Test Fight Self", "Defined$ Self | ValidTgts$ Creature.Other", nil), c)
	if err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if got := g.Card(host).Damage.Marked; got != 3 {
		t.Errorf("host damage = %d, want 3 (target's power)", got)
	}
	if got := g.Card(target).Damage.Marked; got != 2 {
		t.Errorf("target damage = %d, want 2 (host's power)", got)
	}
}

// TestFightEffectTwoTargetsFightEachOther proves fightFighters' own
// "else if haveFighter1" branch: an ability naming ValidTgts$ alone, no
// Defined$, with two chosen targets (TargetMin$/TargetMax$ 2), fights its
// own two targets against each other -- CR 701.12's own "target two
// creatures. They fight" shape.
func TestFightEffectTwoTargetsFightEachOther(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p := g.Players()[0]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life, g.Player(g.Players()[1]).Life = 20, 20
	fighter1 := g.NewCard(creatureDefPT(t, "4", "10"), p, engine.Battlefield)
	fighter2 := g.NewCard(creatureDefPT(t, "1", "10"), p, engine.Battlefield)

	c := engine.NewScriptedController()
	c.QueueTargets([]engine.EntityID{engine.CardEntity(fighter1), engine.CardEntity(fighter2)})

	def := etbFightTriggerDefParams(t, "Test Fight Two Targets", "ValidTgts$ Creature | TargetMin$ 2 | TargetMax$ 2", nil)
	_, err := castETBFight(t, g, p, def, c)
	if err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if got := g.Card(fighter1).Damage.Marked; got != 1 {
		t.Errorf("fighter1 damage = %d, want 1 (fighter2's power)", got)
	}
	if got := g.Card(fighter2).Damage.Marked; got != 4 {
		t.Errorf("fighter2 damage = %d, want 4 (fighter1's power)", got)
	}
}

// TestFightEffectSelfFightDealsDoublePower proves CR 701.12c: a creature
// fighting itself deals damage to itself equal to twice its power.
func TestFightEffectSelfFightDealsDoublePower(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p := g.Players()[0]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life, g.Player(g.Players()[1]).Life = 20, 20

	c := engine.NewScriptedController()
	host := g.NewCard(etbFightTriggerDefParams(t, "Test Fight Itself", "Defined$ Self | ValidTgts$ Creature.Self", nil), p, engine.Hand)
	c.QueueTargets([]engine.EntityID{engine.CardEntity(host)})
	g.Player(p).ManaPool.Add(mana.Green, 1)
	if !g.CastSpell(p, host, c) {
		t.Fatal("CastSpell failed casting a creature with exactly enough mana")
	}
	if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if got := g.Card(host).Damage.Marked; got != 4 {
		t.Errorf("host damage = %d, want 4 (twice its own power of 2)", got)
	}
}

// TestFightEffectNoOpsWithOnlyOneFighter proves fightFighters' own "fewer
// than two fighters resolved" no-op: a lone Defined$ Self with no
// ValidTgts$ at all cannot fight anything.
func TestFightEffectNoOpsWithOnlyOneFighter(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p := g.Players()[0]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life, g.Player(g.Players()[1]).Life = 20, 20

	c := engine.NewScriptedController()
	host, err := castETBFight(t, g, p, etbFightTriggerDefParams(t, "Test Fight Lone", "Defined$ Self", nil), c)
	if err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if got := g.Card(host).Damage.Marked; got != 0 {
		t.Errorf("host damage = %d, want 0 -- a lone fighter cannot fight", got)
	}
}

// TestFightEffectRejectsUnresolvedParam proves fightUnresolvedParams' own
// fail-loud contract (PORT-8/GO-7).
func TestFightEffectRejectsUnresolvedParam(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p := g.Players()[0]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life, g.Player(g.Players()[1]).Life = 20, 20
	target := g.NewCard(creatureDefPT(t, "3", "3"), p, engine.Battlefield)

	c := engine.NewScriptedController()
	c.QueueTargets([]engine.EntityID{engine.CardEntity(target)})

	_, err := castETBFight(t, g, p, etbFightTriggerDefParams(t, "Test Fight Optional", "Defined$ Self | ValidTgts$ Creature.Other | Optional$ True", nil), c)
	if err == nil {
		t.Fatal("ResolveStack succeeded, want an error for unresolved Optional$")
	}
}
