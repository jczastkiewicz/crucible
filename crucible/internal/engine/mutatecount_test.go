package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
	"github.com/jczastkiewicz/crucible/internal/mana"
)

// TestMutateCountsTimesMutated proves Count$TimesMutated (AbilityUtils.java:
// 2107-2108) reads the merged permanent's own counter: a second mutate onto
// the same permanent gains 2 life, the first 1, from "whenever this creature mutates".
func TestMutateCountsTimesMutated(t *testing.T) {
	t.Parallel()
	g, p, _ := newTwoPlayerGame(t)
	c := engine.NewScriptedController()
	host := g.NewCard(copyTestDef(t, "Hemophage", "Creature Vampire", "2", "2", "K:Mutate:G",
		"T:Mode$ Mutates | ValidCard$ Card.Self | Execute$ TrigGain | TriggerZones$ Battlefield",
		"SVar:TrigGain:DB$ GainLife | Defined$ You | LifeAmount$ X", "SVar:X:Count$TimesMutated"), p, engine.Battlefield)
	for _, want := range []int{1, 2} {
		before := g.Player(p).Life
		spell := g.NewCard(copyTestDef(t, "Mutant", "Creature Zombie", "1", "1", "K:Mutate:G"), p, engine.Hand)
		g.Player(p).ManaPool.Add(mana.Green, 1)
		c.QueueOptionNamed("Cast Mutant for its mutate cost {G}, merging with a non-Human creature you own")
		if !g.CastSpell(p, spell, c) {
			t.Fatalf("CastSpell(mutate) failed: %v", g.TakePendingError())
		}
		c.QueueCardChoice([]engine.CardID{host})
		if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
			t.Fatalf("ResolveStack: %v", err)
		}
		if got := g.Player(p).Life - before; got != want {
			t.Fatalf("mutate %d: life gained %d, want %d", want, got, want)
		}
	}
}

// TestSpellMutateSpecMatchesOnlyTheMutateCast proves ValidSA$ Spell.Mutate
// (SpellAbilityProperty.java:115-116): a SpellCast watcher sees the mutate cast
// and not the plain cast of the same card.
func TestSpellMutateSpecMatchesOnlyTheMutateCast(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name   string
		mutate bool
		want   int
	}{{"mutate cast", true, 1}, {"plain cast", false, 0}} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			g, p, _ := newTwoPlayerGame(t)
			c := engine.NewScriptedController()
			g.NewCard(copyTestDef(t, "Under Elf", "Creature Elf", "2", "2"), p, engine.Battlefield)
			g.NewCard(triggerWatcherDef(t, "Watcher", "Mode$ SpellCast | ValidSA$ Spell.Mutate | Execute$ TrigGain", "TrigGain", "DB$ GainLife | Defined$ You | LifeAmount$ 1"), p, engine.Battlefield)
			spell := g.NewCard(copyTestDef(t, "Mutant", "Creature Zombie", "1", "1", "K:Mutate:G"), p, engine.Hand)
			g.Player(p).ManaPool.Add(mana.Green, 3)
			if tc.mutate {
				c.QueueOptionNamed("Cast Mutant for its mutate cost {G}, merging with a non-Human creature you own")
			} else {
				c.QueueOptionNamed("Cast Mutant")
				c.QueuePayGeneric(mana.ShardG)
				c.QueuePayGeneric(mana.ShardG)
			}
			before := g.Player(p).Life
			if !g.CastSpell(p, spell, c) {
				t.Fatalf("CastSpell failed: %v", g.TakePendingError())
			}
			if tc.mutate {
				c.QueueCardChoice([]engine.CardID{spell})
			}
			if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
				t.Fatalf("ResolveStack: %v", err)
			}
			if got := g.Player(p).Life - before; got != tc.want {
				t.Errorf("life gained = %d, want %d", got, tc.want)
			}
		})
	}
}
