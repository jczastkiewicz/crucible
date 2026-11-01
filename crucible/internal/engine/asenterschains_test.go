package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
	"github.com/jczastkiewicz/crucible/internal/mana"
)

// castFromPool casts name from p's hand with the pool holding its mana value in
// black and green, answering generic payment with black, and resolves the
// stack.
func castFromPool(t *testing.T, g *engine.Game, p engine.PlayerID, name string, c *engine.ScriptedController) engine.CardID {
	t.Helper()
	def := corpusCard(t, name)
	card := g.NewCard(def, p, engine.Hand)
	m := def.Faces[0].ManaCost
	g.Player(p).ManaPool.Add(mana.Black, m.CMC())
	g.Player(p).ManaPool.Add(mana.Green, m.CMC())
	for i := 0; i < m.Generic(); i++ {
		c.QueuePayGeneric(mana.ShardB)
	}
	if !g.CastSpell(p, card, c) {
		t.Fatalf("could not cast %s", name)
	}
	if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	if err := g.TakePendingError(); err != nil {
		t.Fatalf("%s: pending %v", name, err)
	}
	return card
}

// K:ETBReplacement:Other chains (CardFactoryUtil.java:515-543) run before the
// card is on the battlefield and write onto it as it enters.
func TestAsEntersChainsWriteOntoTheEnteringCard(t *testing.T) {
	t.Parallel()

	t.Run("Thief of Blood takes every counter", func(t *testing.T) {
		t.Parallel()
		g, p, o := newTwoPlayerGameOn(t, scenarioDB(t))
		a := g.NewCard(corpusCard(t, "Grizzly Bears"), p, engine.Battlefield)
		b := g.NewCard(corpusCard(t, "Grizzly Bears"), o, engine.Battlefield)
		g.Card(a).Counters.Add(engine.P1P1, 2)
		g.Card(b).Counters.Add("CHARGE", 1)
		thief := castFromPool(t, g, p, "Thief of Blood", engine.NewScriptedController())
		if got := g.Card(thief).Counters.Count(engine.P1P1); got != 3 {
			t.Errorf("Thief of Blood has %d +1/+1 counters, want 3", got)
		}
		if g.Card(a).Counters.Count(engine.P1P1) != 0 || g.Card(b).Counters.Count("CHARGE") != 0 {
			t.Error("counters were left on the other permanents")
		}
	})

	t.Run("Jumbo Imp enters with its die roll", func(t *testing.T) {
		t.Parallel()
		g, p, _ := newTwoPlayerGameOn(t, scenarioDB(t))
		imp := castFromPool(t, g, p, "Jumbo Imp", engine.NewScriptedController())
		if n := g.Card(imp).Counters.Count(engine.P1P1); n < 1 || n > 6 || g.Card(imp).Zone != engine.Battlefield {
			t.Errorf("Jumbo Imp has %d counters in %v, want 1 to 6 on the battlefield", n, g.Card(imp).Zone)
		}
	})

	t.Run("Lydari Elephant gets random power and toughness", func(t *testing.T) {
		t.Parallel()
		g, p, _ := newTwoPlayerGameOn(t, scenarioDB(t))
		el := castFromPool(t, g, p, "Lydari Elephant", engine.NewScriptedController())
		pw, okP := g.Card(el).Power()
		th, okT := g.Card(el).Toughness()
		if !okP || !okT || pw < 3 || pw > 7 || th < 3 || th > 7 {
			t.Errorf("Lydari Elephant is %d/%d, want each in 3..7", pw, th)
		}
	})
}
