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

// etbDestroyTriggerDefParams builds a *compile.Card whose own "when CARDNAME
// enters" trigger runs DB$ Destroy with the given params string appended --
// etbLoseLifeTriggerDefParams' own shape (loselifeeffect_test.go).
func etbDestroyTriggerDefParams(t *testing.T, name, extraParams string) *compile.Card {
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
		"Mode$ ChangesZone | Origin$ Any | Destination$ Battlefield | ValidCard$ Card.Self | Execute$ TrigDestroy",
	}
	raw.Faces[0].SVars.Set("TrigDestroy", "DB$ Destroy | "+extraParams)

	c, err := compile.Compile(raw)
	if err != nil {
		t.Fatalf("compile %q: %v", name, err)
	}
	return c
}

// castETBDestroy casts def for p on controller c and resolves the stack --
// castETBSacrifice's own shape (sacrificeeffect_test.go).
func castETBDestroy(t *testing.T, g *engine.Game, p engine.PlayerID, def *compile.Card, c *engine.ScriptedController) (engine.CardID, error) {
	t.Helper()
	g.Player(p).ManaPool.Add(mana.Green, 1)
	creature := g.NewCard(def, p, engine.Hand)
	if !g.CastSpell(p, creature, c) {
		t.Fatal("CastSpell failed casting a creature with exactly enough mana")
	}
	return creature, g.ResolveStack(engine.NewRegistry(), c)
}

// TestDestroyEffectValidTgtsDestroysChosenTarget proves the corpus's own
// dominant real shape: an ability naming ValidTgts$ reads its own chosen
// target directly (a.Targets, targetedOrDefinedCards, defined.go) rather
// than through Defined$, matching royal_assassin.txt's real
// "ValidTgts$ Creature.tapped" shape.
func TestDestroyEffectValidTgtsDestroysChosenTarget(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p := g.Players()[0]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life, g.Player(g.Players()[1]).Life = 20, 20
	target := g.NewCard(creatureDefPT(t, "2", "2"), p, engine.Battlefield)

	c := engine.NewScriptedController()
	c.QueueTargets([]engine.EntityID{engine.CardEntity(target)})

	_, err := castETBDestroy(t, g, p, etbDestroyTriggerDefParams(t, "Test Destroy Tgt", "ValidTgts$ Creature"), c)
	if err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if g.Card(target).Zone != engine.Graveyard {
		t.Errorf("target zone = %v, want Graveyard", g.Card(target).Zone)
	}
}

// TestDestroyEffectDefinedSelfDestroysHost proves an ability naming no
// ValidTgts$ at all falls back to Defined$, defaulting to "Self" the same
// way Java's own getParamOrDefault(definedParam, "Self") does.
func TestDestroyEffectDefinedSelfDestroysHost(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p := g.Players()[0]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life, g.Player(g.Players()[1]).Life = 20, 20

	c := engine.NewScriptedController()
	creature, err := castETBDestroy(t, g, p, etbDestroyTriggerDefParams(t, "Test Destroy Self", "Defined$ Self"), c)
	if err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if g.Card(creature).Zone != engine.Graveyard {
		t.Errorf("host zone = %v, want Graveyard -- Defined$ Self must destroy the host itself", g.Card(creature).Zone)
	}
}

// TestDestroyEffectSkipsIndestructibleCreature proves canBeDestroyed's own
// Indestructible check (Card.canBeDestroyed's own formula) -- the one
// keyword this port checks anywhere (destroyDamagedCreatures' own identical
// reasoning, action.go).
func TestDestroyEffectSkipsIndestructibleCreature(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p := g.Players()[0]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life, g.Player(g.Players()[1]).Life = 20, 20
	target := g.NewCard(indestructibleCreatureDefPT(t, "2", "2"), p, engine.Battlefield)

	c := engine.NewScriptedController()
	c.QueueTargets([]engine.EntityID{engine.CardEntity(target)})

	_, err := castETBDestroy(t, g, p, etbDestroyTriggerDefParams(t, "Test Destroy Indestructible", "ValidTgts$ Creature"), c)
	if err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if g.Card(target).Zone != engine.Battlefield {
		t.Errorf("target zone = %v, want Battlefield -- Indestructible must survive Destroy", g.Card(target).Zone)
	}
}

// TestDestroyEffectRememberDestroyedWritesMemory proves RememberDestroyed$
// writes the destroyed card onto the ability's own host card's Memory --
// TestSacrificeEffectRememberSacrificedWritesMemory's own shape
// (sacrificeeffect_test.go).
func TestDestroyEffectRememberDestroyedWritesMemory(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p := g.Players()[0]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life, g.Player(g.Players()[1]).Life = 20, 20
	target := g.NewCard(creatureDefPT(t, "2", "2"), p, engine.Battlefield)

	c := engine.NewScriptedController()
	c.QueueTargets([]engine.EntityID{engine.CardEntity(target)})

	creature, err := castETBDestroy(t, g, p, etbDestroyTriggerDefParams(t, "Test Destroy Remember", "ValidTgts$ Creature | RememberDestroyed$ True"), c)
	if err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	remembered := g.Card(creature).Memory.Remembered()
	if len(remembered) != 1 || remembered[0] != engine.CardEntity(target) {
		t.Errorf("host's Remembered() = %v, want [%v]", remembered, engine.CardEntity(target))
	}
}

// TestDestroyEffectRejectsUnresolvedParam proves destroyUnresolvedParams'
// own fail-loud contract (PORT-8/GO-7): a real param this port does not
// evaluate errors the whole line rather than destroying the target and
// silently dropping the rest of what the line asked for.
func TestDestroyEffectRejectsUnresolvedParam(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p := g.Players()[0]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life, g.Player(g.Players()[1]).Life = 20, 20
	target := g.NewCard(creatureDefPT(t, "2", "2"), p, engine.Battlefield)

	c := engine.NewScriptedController()
	c.QueueTargets([]engine.EntityID{engine.CardEntity(target)})

	_, err := castETBDestroy(t, g, p, etbDestroyTriggerDefParams(t, "Test Destroy Ultimate", "ValidTgts$ Creature | Ultimate$ True"), c)
	if err == nil {
		t.Fatal("ResolveStack succeeded, want an error for unresolved Ultimate$")
	}
}
