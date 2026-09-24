package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
)

// TestDestroyAllEffectDestroysEveryMatch proves the one shape all 238 real
// lines share: every battlefield permanent matching ValidCards$ is
// destroyed, and nothing else is.
func TestDestroyAllEffectDestroysEveryMatch(t *testing.T) {
	t.Parallel()

	g, p, other := newTwoPlayerGame(t)
	a := g.NewCard(creatureDefPT(t, "2", "2"), other, engine.Battlefield)
	b := g.NewCard(creatureDefPT(t, "3", "3"), other, engine.Battlefield)
	mine := g.NewCard(creatureDefPT(t, "2", "2"), p, engine.Battlefield)

	c := engine.NewScriptedController()
	if _, err := castETBChain(t, g, p, etbChainDef(t, "Test DestroyAll", "DB$ DestroyAll | ValidCards$ Creature.OppCtrl"), c); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	for _, id := range []engine.CardID{a, b} {
		if z := g.Card(id).Zone; z != engine.Graveyard {
			t.Errorf("opponent creature %v zone = %v, want Graveyard", id, z)
		}
	}
	if z := g.Card(mine).Zone; z != engine.Battlefield {
		t.Errorf("own creature zone = %v, want Battlefield", z)
	}
}

// TestDestroyAllEffectSkipsIndestructibleAndRemembers proves canBeDestroyed
// filters first and RememberDestroyed$ records only what actually died.
func TestDestroyAllEffectSkipsIndestructibleAndRemembers(t *testing.T) {
	t.Parallel()

	g, p, other := newTwoPlayerGame(t)
	dies := g.NewCard(creatureDefPT(t, "2", "2"), other, engine.Battlefield)
	lives := g.NewCard(indestructibleCreatureDefPT(t, "2", "2"), other, engine.Battlefield)

	c := engine.NewScriptedController()
	host, err := castETBChain(t, g, p, etbChainDef(t, "Test DestroyAll Remember", "DB$ DestroyAll | ValidCards$ Creature.OppCtrl | RememberDestroyed$ True"), c)
	if err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if z := g.Card(lives).Zone; z != engine.Battlefield {
		t.Errorf("indestructible zone = %v, want Battlefield", z)
	}
	if got := g.Card(host).Memory.Remembered(); len(got) != 1 || got[0] != engine.CardEntity(dies) {
		t.Errorf("Remembered = %v, want [%v]", got, engine.CardEntity(dies))
	}
}

// TestDestroyAllEffectRejectsXInValidCards proves the textual X
// substitution Java performs fails closed here (PORT-2).
func TestDestroyAllEffectRejectsXInValidCards(t *testing.T) {
	t.Parallel()

	g, p, other := newTwoPlayerGame(t)
	g.NewCard(creatureDefPT(t, "2", "2"), other, engine.Battlefield)
	c := engine.NewScriptedController()
	def := etbChainDef(t, "Test DestroyAll X", "DB$ DestroyAll | ValidCards$ Creature.cmcLEX")
	if _, err := castETBChain(t, g, p, def, c); err == nil {
		t.Fatal("ResolveStack succeeded, want an error for X in ValidCards$")
	}
}

// TestDestroyAllEffectValidTgtsNarrowsToTargetedPlayer proves ValidTgts$
// naming a player narrows the sweep to that player's permanents -- Java's
// getFirstTargetedPlayer filter.
func TestDestroyAllEffectValidTgtsNarrowsToTargetedPlayer(t *testing.T) {
	t.Parallel()

	g, p, other := newTwoPlayerGame(t)
	theirs := g.NewCard(creatureDefPT(t, "2", "2"), other, engine.Battlefield)
	mine := g.NewCard(creatureDefPT(t, "2", "2"), p, engine.Battlefield)

	c := engine.NewScriptedController()
	c.QueueTargets([]engine.EntityID{engine.PlayerEntity(other)})
	if _, err := castETBChain(t, g, p, etbChainDef(t, "Test DestroyAll Tgt", "DB$ DestroyAll | ValidTgts$ Player | ValidCards$ Creature"), c); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if z := g.Card(theirs).Zone; z != engine.Graveyard {
		t.Errorf("targeted player's creature zone = %v, want Graveyard", z)
	}
	if z := g.Card(mine).Zone; z != engine.Battlefield {
		t.Errorf("other player's creature zone = %v, want Battlefield", z)
	}
}

// TestDestroyAllEffectRejectsUnresolvedParam proves Optional$ fails closed.
func TestDestroyAllEffectRejectsUnresolvedParam(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGame(t)
	c := engine.NewScriptedController()
	if _, err := castETBChain(t, g, p, etbChainDef(t, "Test DestroyAll Optional", "DB$ DestroyAll | ValidCards$ Creature | Optional$ True"), c); err == nil {
		t.Fatal("ResolveStack succeeded, want an error for unresolved Optional$")
	}
}
