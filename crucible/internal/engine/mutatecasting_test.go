package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
	"github.com/jczastkiewicz/crucible/internal/mana"
)

const mutateTrigger = "T:Mode$ Mutates | ValidCard$ Card.Self | Execute$ TrigDraw | TriggerZones$ Battlefield"

// mutateBoard seats a 2/2 flier for p on the battlefield and a 4/4 trampler
// with Mutate {G} in p's hand, and casts the trampler for its mutate cost with
// the given top choice. It returns (host, spell) with the spell on the stack.
func mutateBoard(t *testing.T, g *engine.Game, p engine.PlayerID, c *engine.ScriptedController) (host, spell engine.CardID) {
	t.Helper()
	host = g.NewCard(copyTestDef(t, "Under Flier", "Creature Elf", "2", "2", "K:Flying"), p, engine.Battlefield)
	spell = g.NewCard(copyTestDef(t, "Over Trampler", "Creature Zombie", "4", "4", "K:Mutate:G", "K:Trample", mutateTrigger,
		"SVar:TrigDraw:DB$ Draw | Defined$ You | NumCards$ 1"), p, engine.Hand)
	g.Player(p).ManaPool.Add(mana.Green, 1)
	c.QueueOptionNamed("Cast Over Trampler for its mutate cost {G}, merging with a non-Human creature you own")
	if !g.CastSpell(p, spell, c) {
		t.Fatalf("CastSpell(mutate) failed: %v", g.TakePendingError())
	}
	return host, spell
}

// TestMutateMergesOverTheTargetAndJoinsAbilities proves the spell merges on top:
// the permanent keeps the target's id, takes the top's power and toughness, has
// the abilities of both, the spell card sits in Merged, and the Mutates trigger
// fires once for the target (MutateEffect.java).
func TestMutateMergesOverTheTargetAndJoinsAbilities(t *testing.T) {
	t.Parallel()
	g, p, _ := newTwoPlayerGame(t)
	libraryCards(t, g, p, 2)
	c := engine.NewScriptedController()
	host, spell := mutateBoard(t, g, p, c)
	c.QueueCardChoice([]engine.CardID{spell})
	if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	h := g.Card(host)
	if pw, _ := h.Power(); pw != 4 {
		t.Errorf("merged power %d, want the top's 4", pw)
	}
	if !h.HasKeyword("Flying") || !h.HasKeyword("Trample") {
		t.Errorf("merged keywords lack Flying or Trample: %v", h.KeywordLines())
	}
	if z := g.Card(spell).Zone; z != engine.Merged {
		t.Errorf("spell card in %v, want Merged", z)
	}
	if n := g.Zone(engine.Hand, p).Len(); n != 1 {
		t.Errorf("hand size %d, want 1 (the Mutates trigger drew)", n)
	}
}

// TestMutateUnderKeepsTheTargetOnTop proves the under choice: the permanent keeps
// the target's power and gains the spell's abilities.
func TestMutateUnderKeepsTheTargetOnTop(t *testing.T) {
	t.Parallel()
	g, p, _ := newTwoPlayerGame(t)
	libraryCards(t, g, p, 2)
	c := engine.NewScriptedController()
	host, _ := mutateBoard(t, g, p, c)
	c.QueueCardChoice([]engine.CardID{host})
	if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	h := g.Card(host)
	if pw, _ := h.Power(); pw != 2 {
		t.Errorf("merged power %d, want the target's 2", pw)
	}
	if !h.HasKeyword("Trample") {
		t.Error("the under spell's Trample was not joined")
	}
}

// TestMutateSplitsOnLeavingTheBattlefield proves every component goes to its
// owner's graveyard when the merged permanent dies (GameAction.java:400-528).
func TestMutateSplitsOnLeavingTheBattlefield(t *testing.T) {
	t.Parallel()
	g, p, _ := newTwoPlayerGame(t)
	libraryCards(t, g, p, 2)
	c := engine.NewScriptedController()
	host, spell := mutateBoard(t, g, p, c)
	c.QueueCardChoice([]engine.CardID{spell})
	if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	g.Move(host, engine.Graveyard, p)
	for _, id := range []engine.CardID{host, spell} {
		if z := g.Card(id).Zone; z != engine.Graveyard {
			t.Errorf("component %d in %v, want Graveyard", id, z)
		}
	}
	if pw, _ := g.Card(host).Power(); pw != 2 {
		t.Errorf("split target power %d, want its own 2", pw)
	}
}

// TestMutateWithAGoneTargetResolvesAsACreature proves CR 702.140b: the target
// left, so the spell enters the battlefield on its own (MagicStack.java:628).
func TestMutateWithAGoneTargetResolvesAsACreature(t *testing.T) {
	t.Parallel()
	g, p, _ := newTwoPlayerGame(t)
	c := engine.NewScriptedController()
	host, spell := mutateBoard(t, g, p, c)
	g.Move(host, engine.Graveyard, p)
	if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if z := g.Card(spell).Zone; z != engine.Battlefield {
		t.Errorf("spell in %v, want Battlefield", z)
	}
}
